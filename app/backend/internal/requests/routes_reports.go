package api

import (
	"errors"
	"net/http"
	"slices"
	"strconv"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
)

// Reports (#95): anyone signed in reports a problem with a title in the
// Library; the owner and admins (manage_library) see every report and mark
// it fixed or open it again; everyone else sees only their own.
func (a *API) reportRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/reports", a.listReports)
	m.HandleFunc("POST /api/v1/reports", a.makeReport)
	m.HandleFunc("POST /api/v1/reports/{id}/fix", a.fixReport)
	m.HandleFunc("POST /api/v1/reports/{id}/reopen", a.reopenReport)
}

// mayManageReports says whether the signed-in person sees and closes every
// report (manage_library; everyone without sign-in in tests).
func mayManageReports(r *http.Request) bool {
	who, ok := webserver.From(r.Context())
	return !ok || who.Has(webserver.PermManageLibrary)
}

func reportError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, requests.ErrReportKind), errors.Is(err, requests.ErrReportPart), errors.Is(err, requests.ErrNoteLength):
		webserver.Fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, requests.ErrNotInLibrary):
		webserver.Fail(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, requests.ErrReportedAlready), errors.Is(err, requests.ErrReportFixed), errors.Is(err, requests.ErrReportOpen):
		webserver.Fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, requests.ErrTooManyReports):
		webserver.Fail(w, http.StatusTooManyRequests, err.Error())
	case errors.Is(err, database.ErrNotFound):
		webserver.Fail(w, http.StatusNotFound, "no such report")
	default:
		webserver.Internal(w, "api", err)
	}
}

// reportLinks turns the stored poster into a link and marks the reader's own.
func (a *API) reportLinks(r *http.Request, vs []requests.ReportView) []requests.ReportView {
	me := a.person(r).UserID
	for i := range vs {
		if vs[i].Poster != "" {
			vs[i].Poster = artworkLinks([]string{vs[i].Poster})[0]
		}
		vs[i].Mine = vs[i].MadeBy != nil && vs[i].MadeBy.ID == me
	}
	return vs
}

// listReports lists every report for managers, else the person's own.
// ?state=open|fixed, ?title_id=<id> and ?mine=1 narrow the list.
func (a *API) listReports(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	state := q.Get("state")
	if state != "" && state != domain.ReportOpen && state != domain.ReportFixed {
		webserver.Fail(w, http.StatusBadRequest, "state must be open or fixed")
		return
	}
	var titleID int64
	if s := q.Get("title_id"); s != "" {
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			webserver.Fail(w, http.StatusBadRequest, "title_id must be a number")
			return
		}
		titleID = id
	}
	all := mayManageReports(r) && q.Get("mine") != "1"
	vs, err := a.s.Asks.ListReports(r.Context(), a.person(r), all, state, titleID, limit(r, 200))
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	// A title above the reader's Kid limit stays hidden, even their own
	// report of it after the limit was lowered.
	vs = slices.DeleteFunc(vs, func(v requests.ReportView) bool { return !metadata.RatingAllows(a.kidMax(r), v.TitleKind, v.Rating) })
	out := map[string]any{"reports": a.reportLinks(r, vs)}
	if mayManageReports(r) {
		n, _ := requests.OpenReports(r.Context(), a.s.Store.Read())
		out["open"] = n
	}
	webserver.WriteJSON(w, http.StatusOK, out)
}

const reportHelp = `send {"title_id": <Library title id>, "kind": "picture|sound|subtitles|wont_play|other", "note": "..."}; "season", "item_id" (an episode or track), "album_id" and "version_id" narrow it`

func (a *API) makeReport(w http.ResponseWriter, r *http.Request) {
	var in requests.ReportInput
	if !decode(w, r, &in, reportHelp) {
		return
	}
	// The same title check as GET /titles/{id}: no such title and a title
	// above a Kid's limit answer alike, and a title of a part that is off waits.
	var t mediafiles.Title
	if err := database.Get(r.Context(), a.s.Store.Read(), "titles", in.TitleID, &t); err != nil || !a.titleAllowed(r, t) {
		if err != nil && !errors.Is(err, database.ErrNotFound) {
			webserver.Internal(w, "api", err)
			return
		}
		webserver.Fail(w, http.StatusNotFound, "no such Title")
		return
	}
	if p, known := parts.ForTitleKind(t.Kind); known && !a.partIsOn(p) {
		webserver.WriteJSON(w, http.StatusConflict, partOffBody(parts.Of(p)))
		return
	}
	v, err := a.s.Asks.Report(r.Context(), a.person(r), in)
	if err != nil {
		reportError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusCreated, map[string]any{"report": a.reportLinks(r, []requests.ReportView{v})[0]})
}

func reportID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		webserver.Fail(w, http.StatusNotFound, "no such report")
		return 0, false
	}
	return id, true
}

func (a *API) fixReport(w http.ResponseWriter, r *http.Request) {
	id, ok := reportID(w, r)
	if !ok {
		return
	}
	var body struct {
		Reply string `json:"reply"`
	}
	if r.ContentLength != 0 && !decode(w, r, &body, `send {} or {"reply": "what was done"}`) {
		return
	}
	v, err := a.s.Asks.FixReport(r.Context(), a.person(r), id, body.Reply)
	if err != nil {
		reportError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"report": a.reportLinks(r, []requests.ReportView{v})[0]})
}

func (a *API) reopenReport(w http.ResponseWriter, r *http.Request) {
	id, ok := reportID(w, r)
	if !ok {
		return
	}
	v, err := a.s.Asks.ReopenReport(r.Context(), a.person(r), id)
	if err != nil {
		reportError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"report": a.reportLinks(r, []requests.ReportView{v})[0]})
}
