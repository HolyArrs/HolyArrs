package discover

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
)

// HiddenRoutes registers the hide list: owners and admins hide a title or
// a genre from browsing, for everyone or one person, and show it again.
func (h *Handlers) HiddenRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/hidden", h.listHidden)
	m.HandleFunc("POST /api/v1/hidden", h.addHidden)
	m.HandleFunc("DELETE /api/v1/hidden/{id}", h.showAgain)
}

// listHidden answers GET /api/v1/hidden[?user_id=]: every hide, oldest
// first; user_id=0 keeps the household's, user_id=<id> one person's.
func (h *Handlers) listHidden(w http.ResponseWriter, r *http.Request) {
	set, err := hiddenFor(r.Context(), h.Store)
	if err != nil {
		webserver.Internal(w, "discover", err)
		return
	}
	out := set.rows
	if v := r.URL.Query().Get("user_id"); v != "" {
		user, err := strconv.ParseInt(v, 10, 64)
		if err != nil || user < 0 {
			webserver.Fail(w, http.StatusBadRequest, "user_id must be 0 (everyone) or a person's id")
			return
		}
		out = []Hide{}
		for _, x := range set.rows {
			if x.UserID == user {
				out = append(out, x)
			}
		}
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"hidden": out})
}

const hideHelp = `send {"kind": "movie" or "show", "tmdb_id": "<TMDB id>", "name": "<title>"} to hide a title, or {"kind": ..., "genre": "<genre>"} to hide a genre; add "user_id": <id> to hide it for one person only`

// addHidden answers POST /api/v1/hidden: 201 with the new hide, or 200
// with the same one when it was already there.
func (h *Handlers) addHidden(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Kind   string `json:"kind"`
		TMDBID string `json:"tmdb_id"`
		Genre  string `json:"genre"`
		Name   string `json:"name"`
		UserID int64  `json:"user_id"`
	}
	if !webserver.Decode(w, r, &body, hideHelp) {
		return
	}
	x := Hide{Kind: body.Kind, TMDBID: strings.TrimSpace(body.TMDBID), Genre: strings.TrimSpace(body.Genre),
		Name: strings.TrimSpace(body.Name), UserID: body.UserID}
	if msg := checkHide(x); msg != "" {
		webserver.Fail(w, http.StatusBadRequest, msg)
		return
	}
	if x.Genre != "" {
		x.Name = ""
	}
	got, created, err := insertHide(r.Context(), h.Store, x, h.Actor(r))
	if errors.Is(err, errHideUser) {
		webserver.Fail(w, http.StatusBadRequest, "no such person")
		return
	}
	if err != nil {
		webserver.Internal(w, "discover", err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	webserver.WriteJSON(w, status, got)
}

// checkHide says what is wrong with x ("" when nothing).
func checkHide(x Hide) string {
	if x.Kind != "movie" && x.Kind != "show" {
		return "kind must be movie or show"
	}
	if (x.TMDBID == "") == (x.Genre == "") {
		return hideHelp
	}
	if x.TMDBID != "" {
		if n, err := strconv.ParseInt(x.TMDBID, 10, 64); err != nil || n <= 0 || strconv.FormatInt(n, 10) != x.TMDBID {
			return "tmdb_id must be a TMDB number"
		}
	}
	if x.Genre != "" && !knownGenre(x.Kind, x.Genre) {
		return "genre must be one of the " + x.Kind + " genres"
	}
	if len(x.Name) > 300 {
		return "name must be 300 characters or fewer"
	}
	if x.UserID < 0 {
		return "user_id must be a person's id, or 0 for everyone"
	}
	return ""
}

// moreGenres are TMDB genres a title page can show that the genre picker
// leaves out.
var moreGenres = map[string][]string{"movie": {"TV Movie"}, "show": {"News", "Soap", "Talk"}}

// knownGenre: genre is one of TMDB's genres for kind.
func knownGenre(kind, genre string) bool {
	for _, g := range engine.Genres[kind] {
		if g == genre {
			return true
		}
	}
	return slices.Contains(moreGenres[kind], genre)
}

// showAgain answers DELETE /api/v1/hidden/{id} (Undo and Show again).
func (h *Handlers) showAgain(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		webserver.Fail(w, http.StatusBadRequest, "id must be a hide's id")
		return
	}
	found, err := deleteHide(r.Context(), h.Store, id)
	if err != nil {
		webserver.Internal(w, "discover", err)
		return
	}
	if !found {
		webserver.Fail(w, http.StatusNotFound, "no such hide")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
