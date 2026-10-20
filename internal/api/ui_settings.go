package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/CoderLuii/holyarrs/internal/store"
)

// App-wide look settings everyone's browser reads: today only whether
// trailers play by themselves (heroes and hover cards). Stored in the
// this_app settings group.

const settingAutoplay = "autoplay_trailers"

// UISettings is the answer of GET /settings/ui.
type UISettings struct {
	// AutoplayTrailers lets heroes and hover cards play a muted trailer by
	// themselves (never for Kid accounts or with Reduce motion). Default on.
	AutoplayTrailers bool `json:"autoplay_trailers"`
}

func (a *API) uiSettings(ctx context.Context) UISettings {
	var v string
	_ = a.s.Store.Read().QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = 'this_app' AND key = ?", settingAutoplay).Scan(&v)
	return UISettings{AutoplayTrailers: v != "off"}
}

func (a *API) getUISettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.uiSettings(r.Context()))
}

func (a *API) putUISettings(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AutoplayTrailers *bool `json:"autoplay_trailers"`
	}
	if !decode(w, r, &in, `send {"autoplay_trailers": true}`) {
		return
	}
	if in.AutoplayTrailers != nil {
		v := map[bool]string{true: "on", false: "off"}[*in.AutoplayTrailers]
		now := store.FormatTime(time.Now())
		by, _ := json.Marshal(actor(r))
		err := a.s.Store.Write(r.Context(), func(tx *sql.Tx) error {
			_, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('this_app', ?, ?, ?, ?, ?, ?)
			ON CONFLICT (grp, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
				settingAutoplay, v, now, string(by), now, string(by))
			return err
		})
		if err != nil {
			internal(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, a.uiSettings(r.Context()))
}
