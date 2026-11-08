package shows

import (
	"net/http"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// SeasonHandlers serves a show's seasons and the show settings.
type SeasonHandlers struct {
	Store *database.Store
	Lib   *library.Service
	// TitleID reads the {id} Title, answering 404 itself.
	TitleID func(w http.ResponseWriter, r *http.Request) (mediafiles.Title, bool)
	// Fail answers a show or library error.
	Fail func(w http.ResponseWriter, err error)
}

// Routes registers the seasons and show settings routes.
func (h *SeasonHandlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/titles/{id}/seasons", h.listSeasons)
	m.HandleFunc("PUT /api/v1/titles/{id}/seasons/{number}", h.setSeason)
	m.HandleFunc("GET /api/v1/settings/shows", h.getShowSettings)
	m.HandleFunc("PUT /api/v1/settings/shows", h.putShowSettings)
}

// KeepHelp is the help text for a keep_up_to_date change.
const KeepHelp = `send {"keep_up_to_date": true or false}`

// ShowID reads the {id} Title through titleID and answers 404 when it is not a show.
func ShowID(titleID func(http.ResponseWriter, *http.Request) (mediafiles.Title, bool), w http.ResponseWriter, r *http.Request) (mediafiles.Title, bool) {
	t, ok := titleID(w, r)
	if ok && t.Kind != "show" {
		webserver.Fail(w, http.StatusNotFound, "that Title is not a show")
		return t, false
	}
	return t, ok
}

func (h *SeasonHandlers) listSeasons(w http.ResponseWriter, r *http.Request) {
	t, ok := ShowID(h.TitleID, w, r)
	if !ok {
		return
	}
	ss, err := Seasons(r.Context(), h.Store.Read(), t.ID, time.Now())
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"seasons": ss})
}

func (h *SeasonHandlers) setSeason(w http.ResponseWriter, r *http.Request) {
	t, ok := ShowID(h.TitleID, w, r)
	if !ok {
		return
	}
	n, err := strconv.Atoi(r.PathValue("number"))
	if err != nil || n < 0 {
		webserver.Fail(w, http.StatusNotFound, "no such season")
		return
	}
	var body struct {
		KeepUpToDate *bool `json:"keep_up_to_date"`
	}
	if !webserver.Decode(w, r, &body, KeepHelp) {
		return
	}
	if body.KeepUpToDate == nil {
		webserver.Fail(w, http.StatusBadRequest, KeepHelp)
		return
	}
	if err := New(h.Lib).SetSeason(r.Context(), t.ID, n, *body.KeepUpToDate, actor(r)); err != nil {
		h.Fail(w, err)
		return
	}
	ss, err := Seasons(r.Context(), h.Store.Read(), t.ID, time.Now())
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	for _, s := range ss {
		if s.Number == n {
			webserver.WriteJSON(w, http.StatusOK, s)
			return
		}
	}
	webserver.Fail(w, http.StatusNotFound, "no such season")
}

func (h *SeasonHandlers) getShowSettings(w http.ResponseWriter, r *http.Request) {
	d, err := library.ReadShowDefaults(r.Context(), h.Store.Read())
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, d)
}

func (h *SeasonHandlers) putShowSettings(w http.ResponseWriter, r *http.Request) {
	d, err := library.ReadShowDefaults(r.Context(), h.Store.Read())
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	if !webserver.Decode(w, r, &d, "send any of season_choice, new_seasons, episode_order, details_source, keep_specials, episode_wait_minutes") {
		return
	}
	if err := SetShowDefaults(r.Context(), h.Store, d, actor(r)); err != nil {
		h.Fail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, d)
}
