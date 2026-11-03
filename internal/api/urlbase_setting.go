package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/CoderLuii/holyarrs/internal/store"
	"github.com/CoderLuii/holyarrs/internal/urlbase"
)

// The URL base saved from Settings; HOLYARRS_URL_BASE or -url-base win
// over it, and it takes effect on the next start.
const urlBaseGroup, urlBaseKey = "this_app", "url_base"

// SavedURLBase reads the saved base for startup. A missing or bad value is
// no base, so a wrong save never locks anyone out after a restart.
func SavedURLBase(ctx context.Context, db *sql.DB) string {
	var raw string
	if err := db.QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = ? AND key = ?", urlBaseGroup, urlBaseKey).Scan(&raw); err != nil {
		return ""
	}
	var v string
	if json.Unmarshal([]byte(raw), &v) != nil {
		return ""
	}
	b, err := urlbase.Clean(v)
	if err != nil {
		return ""
	}
	return b
}

type urlBaseAnswer struct {
	Saved         string `json:"saved"`
	InUse         string `json:"in_use"`
	RestartNeeded bool   `json:"restart_needed"`
}

func (a *API) urlBaseAnswer(ctx context.Context) urlBaseAnswer {
	saved := SavedURLBase(ctx, a.s.Store.Read())
	return urlBaseAnswer{Saved: saved, InUse: a.s.URLBase, RestartNeeded: saved != a.s.URLBase}
}

func (a *API) getURLBase(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.urlBaseAnswer(r.Context()))
}

func (a *API) putURLBase(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URLBase string `json:"url_base"`
	}
	if !decode(w, r, &body, `send {"url_base": "/holyarrs"}, or "" for none`) {
		return
	}
	b, err := urlbase.Clean(body.URLBase)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	raw, _ := json.Marshal(b)
	by, _ := json.Marshal(actor(r))
	now := store.FormatTime(time.Now())
	err = a.s.Store.Write(r.Context(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (grp, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
			urlBaseGroup, urlBaseKey, string(raw), now, string(by), now, string(by))
		return err
	})
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, a.urlBaseAnswer(r.Context()))
}
