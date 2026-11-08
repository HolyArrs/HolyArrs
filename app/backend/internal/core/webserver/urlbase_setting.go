package webserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// The URL base saved from Settings; HOLYARRS_URL_BASE or -url-base win
// over it, and it takes effect on the next start.
const urlBaseGroup, urlBaseSettingKey = "this_app", "url_base"

// SavedURLBase reads the saved base for startup. A missing or bad value is
// no base, so a wrong save never locks anyone out after a restart.
func SavedURLBase(ctx context.Context, db database.Querier) string {
	raw, found, err := database.SettingFound(ctx, db, urlBaseGroup, urlBaseSettingKey)
	if err != nil || !found {
		return ""
	}
	var v string
	if json.Unmarshal([]byte(raw), &v) != nil {
		return ""
	}
	b, err := CleanURLBase(v)
	if err != nil {
		return ""
	}
	return b
}

// URLBaseHandlers serve the URL base setting. InUse is the base this run
// started with.
type URLBaseHandlers struct {
	Store *database.Store
	InUse string
}

// Routes registers the URL base setting routes.
func (h *URLBaseHandlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/settings/url-base", h.get)
	m.HandleFunc("PUT /api/v1/settings/url-base", h.put)
}

type urlBaseAnswer struct {
	Saved         string `json:"saved"`
	InUse         string `json:"in_use"`
	RestartNeeded bool   `json:"restart_needed"`
}

func (h *URLBaseHandlers) answer(ctx context.Context) urlBaseAnswer {
	saved := SavedURLBase(ctx, h.Store.Read())
	return urlBaseAnswer{Saved: saved, InUse: h.InUse, RestartNeeded: saved != h.InUse}
}

func (h *URLBaseHandlers) get(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, h.answer(r.Context()))
}

func (h *URLBaseHandlers) put(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URLBase string `json:"url_base"`
	}
	if !Decode(w, r, &body, `send {"url_base": "/holyarrs"}, or "" for none`) {
		return
	}
	b, err := CleanURLBase(body.URLBase)
	if err != nil {
		Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	raw, _ := json.Marshal(b)
	by := database.Actor{Kind: "user", ID: 1} // no caller only in tests
	if _, ok := From(r.Context()); ok {
		by = Actor(r.Context())
	}
	err = h.Store.Write(r.Context(), func(tx *sql.Tx) error {
		return database.PutSetting(tx, urlBaseGroup, urlBaseSettingKey, string(raw), by, time.Now())
	})
	if err != nil {
		Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, h.answer(r.Context()))
}
