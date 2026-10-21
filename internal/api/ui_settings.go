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

const (
	settingAutoplay = "autoplay_trailers"
	// SettingYTDLPUpdate turns the weekly yt-dlp update on or off.
	SettingYTDLPUpdate = "ytdlp_auto_update"
)

// UISettings is the answer of GET /settings/ui.
type UISettings struct {
	// AutoplayTrailers lets heroes and hover cards play a muted trailer by
	// themselves (never for Kid accounts or with Reduce motion). Default on.
	AutoplayTrailers bool `json:"autoplay_trailers"`
	// KeepYTDLPUpdated: "Keep yt-dlp up to date", the weekly update from
	// yt-dlp's official releases. Default on.
	KeepYTDLPUpdated bool `json:"keep_ytdlp_updated"`
	// SigninLibraryArt: "Show Library art on the sign-in page" (Library
	// titles first). Off shows trending titles only. Default off.
	SigninLibraryArt bool `json:"signin_library_art"`
}

func (a *API) uiSettings(ctx context.Context) UISettings {
	var v, u string
	_ = a.s.Store.Read().QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = 'this_app' AND key = ?", settingAutoplay).Scan(&v)
	_ = a.s.Store.Read().QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = 'this_app' AND key = ?", SettingYTDLPUpdate).Scan(&u)
	var l string
	_ = a.s.Store.Read().QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = 'this_app' AND key = ?", settingSigninLibraryArt).Scan(&l)
	return UISettings{AutoplayTrailers: v != "off", KeepYTDLPUpdated: u != "off", SigninLibraryArt: l == "on"}
}

// KeepYTDLPUpdated reads "Keep yt-dlp up to date" (for the weekly job).
func (a *API) KeepYTDLPUpdated(ctx context.Context) bool { return a.uiSettings(ctx).KeepYTDLPUpdated }

func (a *API) getUISettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.uiSettings(r.Context()))
}

func (a *API) putUISettings(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AutoplayTrailers *bool `json:"autoplay_trailers"`
		KeepYTDLPUpdated *bool `json:"keep_ytdlp_updated"`
		SigninLibraryArt *bool `json:"signin_library_art"`
	}
	if !decode(w, r, &in, `send {"autoplay_trailers": true, "keep_ytdlp_updated": true, "signin_library_art": true}`) {
		return
	}
	for key, on := range map[string]*bool{settingAutoplay: in.AutoplayTrailers, SettingYTDLPUpdate: in.KeepYTDLPUpdated, settingSigninLibraryArt: in.SigninLibraryArt} {
		if on == nil {
			continue
		}
		v := map[bool]string{true: "on", false: "off"}[*on]
		now := store.FormatTime(time.Now())
		by, _ := json.Marshal(actor(r))
		err := a.s.Store.Write(r.Context(), func(tx *sql.Tx) error {
			_, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('this_app', ?, ?, ?, ?, ?, ?)
			ON CONFLICT (grp, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
				key, v, now, string(by), now, string(by))
			return err
		})
		if err != nil {
			internal(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, a.uiSettings(r.Context()))
}
