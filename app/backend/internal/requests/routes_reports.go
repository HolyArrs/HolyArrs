package requests

import (
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Reports: anyone signed in reports a problem with a title in the
// Library; the owner and admins (manage_library) see every report and mark
// it fixed or open it again; everyone else sees only their own.
type ReportHandlers struct {
	Svc   *Service
	Store *database.Store
	// Person is the signed-in person as Requests sees them.
	Person func(r *http.Request) Person
	// KidMax is the age limit of the signed-in Kid, or "" for everyone else.
	KidMax func(r *http.Request) string
	// TitleGate answers 404 (no such or hidden title) or 409 (its part is
	// off) for a Library title and says false; true lets the report through.
	TitleGate func(w http.ResponseWriter, r *http.Request, titleID int64) bool
}

// Routes registers the report routes.
func (h *ReportHandlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/reports", h.listReports)
	m.HandleFunc("POST /api/v1/reports", h.makeReport)
	m.HandleFunc("POST /api/v1/reports/{id}/fix", h.fixReport)
	m.HandleFunc("POST /api/v1/reports/{id}/reopen", h.reopenReport)
}

// MayManageReports says whether the signed-in person sees and closes every
// report (manage_library; everyone without sign-in in tests).
func MayManageReports(r *http.Request) bool {
	who, ok := webserver.From(r.Context())
	return !ok || who.Has(webserver.PermManageLibrary)
}

func reportError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrReportKind), errors.Is(err, ErrReportPart), errors.Is(err, ErrNoteLength):
		webserver.Fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrNotInLibrary):
		webserver.Fail(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ErrReportedAlready), errors.Is(err, ErrReportFixed), errors.Is(err, ErrReportOpen):
		webserver.Fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrTooManyReports):
		webserver.Fail(w, http.StatusTooManyRequests, err.Error())
	case errors.Is(err, database.ErrNotFound):
		webserver.Fail(w, http.StatusNotFound, "no such report")
	default:
		webserver.Internal(w, "requests", err)
	}
}

// reportLinks turns the stored poster into a link and marks the reader's own.
func (h *ReportHandlers) reportLinks(r *http.Request, vs []ReportView) []ReportView {
	me := h.Person(r).UserID
	for i := range vs {
		if vs[i].Poster != "" {
			vs[i].Poster = reportArtworkLink(vs[i].Poster)
		}
		vs[i].Mine = vs[i].MadeBy != nil && vs[i].MadeBy.ID == me
	}
	return vs
}

// listReports lists every report for managers, else the person's own.
// ?state=open|fixed, ?title_id=<id> and ?mine=1 narrow the list.
func (h *ReportHandlers) listReports(w http.ResponseWriter, r *http.Request) {
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
	all := MayManageReports(r) && q.Get("mine") != "1"
	vs, err := h.Svc.ListReports(r.Context(), h.Person(r), all, state, titleID, reportLimit(r, 200))
	if err != nil {
		webserver.Internal(w, "requests", err)
		return
	}
	// A title above the reader's Kid limit stays hidden, even their own
	// report of it after the limit was lowered.
	vs = slices.DeleteFunc(vs, func(v ReportView) bool { return !metadata.RatingAllows(h.KidMax(r), v.TitleKind, v.Rating) })
	out := map[string]any{"reports": h.reportLinks(r, vs)}
	if MayManageReports(r) {
		n, _ := OpenReports(r.Context(), h.Store.Read())
		out["open"] = n
	}
	webserver.WriteJSON(w, http.StatusOK, out)
}

const reportHelp = `send {"title_id": <Library title id>, "kind": "picture|sound|subtitles|wont_play|other", "note": "..."}; "season", "item_id" (an episode or track), "album_id" and "version_id" narrow it`

func (h *ReportHandlers) makeReport(w http.ResponseWriter, r *http.Request) {
	var in ReportInput
	if !webserver.Decode(w, r, &in, reportHelp) {
		return
	}
	if !h.TitleGate(w, r, in.TitleID) {
		return
	}
	v, err := h.Svc.Report(r.Context(), h.Person(r), in)
	if err != nil {
		reportError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusCreated, map[string]any{"report": h.reportLinks(r, []ReportView{v})[0]})
}

func reportID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		webserver.Fail(w, http.StatusNotFound, "no such report")
		return 0, false
	}
	return id, true
}

func (h *ReportHandlers) fixReport(w http.ResponseWriter, r *http.Request) {
	id, ok := reportID(w, r)
	if !ok {
		return
	}
	var body struct {
		Reply string `json:"reply"`
	}
	if r.ContentLength != 0 && !webserver.Decode(w, r, &body, `send {} or {"reply": "what was done"}`) {
		return
	}
	v, err := h.Svc.FixReport(r.Context(), h.Person(r), id, body.Reply)
	if err != nil {
		reportError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"report": h.reportLinks(r, []ReportView{v})[0]})
}

func (h *ReportHandlers) reopenReport(w http.ResponseWriter, r *http.Request) {
	id, ok := reportID(w, r)
	if !ok {
		return
	}
	v, err := h.Svc.ReopenReport(r.Context(), h.Person(r), id)
	if err != nil {
		reportError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"report": h.reportLinks(r, []ReportView{v})[0]})
}

// reportArtworkLink turns a stored poster ("poster:<cache name>") into a
// link the web app loads; anything else is kept.
func reportArtworkLink(a string) string {
	if name, ok := strings.CutPrefix(a, "poster:"); ok && name != "" {
		return "/api/v1/artwork/" + url.PathEscape(name)
	}
	return a
}

func reportLimit(r *http.Request, def int) int {
	n, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || n <= 0 || n > 500 {
		return def
	}
	return n
}
