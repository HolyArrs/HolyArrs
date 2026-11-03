package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/CoderLuii/holyarrs/internal/auth"
	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// App-wide look settings everyone's browser reads: today only whether
// trailers play by themselves (heroes and hover cards). Stored in the
// this_app settings group.

const (
	settingAutoplay = "autoplay_trailers"
	// SettingYTDLPUpdate was the weekly yt-dlp update's switch. Trailers use
	// HolyArrs's own YouTube reader now, so it is kept but does nothing.
	SettingYTDLPUpdate = "ytdlp_auto_update"
	// settingCountryCount is "Count this install" (the metadata server's
	// per-country install total).
	settingCountryCount = "install_country_count"
)

// UISettings is the answer of GET /settings/ui.
type UISettings struct {
	// AutoplayTrailers lets heroes and hover cards play a muted trailer by
	// themselves (never for Kid accounts or with Reduce motion). Default on.
	AutoplayTrailers bool `json:"autoplay_trailers"`
	// KeepYTDLPUpdated was "Keep yt-dlp up to date". It is still accepted
	// and answered so older pages keep working, but it is ignored: trailers
	// use HolyArrs's own YouTube reader, which updates with HolyArrs.
	KeepYTDLPUpdated bool `json:"keep_ytdlp_updated"`
	// SigninLibraryArt: "Show Library art on the sign-in page" (Library
	// titles first). Off shows trending titles only. Default off.
	SigninLibraryArt bool `json:"signin_library_art"`
	// ArtworkCacheGB is the picture cache's size: 1, 2, 5, 10 or 20 GB.
	// Default 5 (decision 0002).
	ArtworkCacheGB int `json:"artwork_cache_gb"`
	// History keep times in days (0 keeps forever): routine work (searches,
	// refreshes, scans) and everything else. Defaults 30 and 0.
	HistoryRoutineDays int `json:"history_routine_days"`
	HistoryOtherDays   int `json:"history_other_days"`
	// CountInstall: "Count this install" in the metadata server's
	// per-country install total. Default on.
	CountInstall bool `json:"count_install"`
}

// settingArtworkCache is the picture cache size in GB; artworkCacheSizes the choices.
const settingArtworkCache = "artwork_cache_gb"

var artworkCacheSizes = map[int]bool{1: true, 2: true, 5: true, 10: true, 20: true}

// applyArtworkCap sets the picture cache's cap from Settings.
func (a *API) applyArtworkCap(ctx context.Context) {
	if a.s.Library != nil && a.s.Library.Artwork != nil {
		a.s.Library.Artwork.SetCap(int64(a.uiSettings(ctx).ArtworkCacheGB) << 30)
	}
}

func (a *API) uiSettings(ctx context.Context) UISettings {
	var v, u string
	_ = a.s.Store.Read().QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = 'this_app' AND key = ?", settingAutoplay).Scan(&v)
	_ = a.s.Store.Read().QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = 'this_app' AND key = ?", SettingYTDLPUpdate).Scan(&u)
	var l string
	_ = a.s.Store.Read().QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = 'this_app' AND key = ?", settingSigninLibraryArt).Scan(&l)
	var g string
	_ = a.s.Store.Read().QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = 'this_app' AND key = ?", settingArtworkCache).Scan(&g)
	gb, err := strconv.Atoi(g)
	if err != nil || !artworkCacheSizes[gb] {
		gb = int(library.DefaultArtworkCap >> 30)
	}
	k := store.ReadHistoryKeep(ctx, a.s.Store.Read())
	return UISettings{AutoplayTrailers: v != "off", KeepYTDLPUpdated: u != "off", SigninLibraryArt: l == "on", ArtworkCacheGB: gb,
		HistoryRoutineDays: k.RoutineDays, HistoryOtherDays: k.OtherDays, CountInstall: CountInstall(ctx, a.s.Store.Read())}
}

// CountInstall reads the stored "Count this install" (on unless switched
// off). The app sets it on the metadata server's requests before any Job
// runs, so no request goes out counted after the owner switched it off.
func CountInstall(ctx context.Context, db *sql.DB) bool {
	var c string
	_ = db.QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = 'this_app' AND key = ?", settingCountryCount).Scan(&c)
	return c != "off"
}

// ApplyCountryCount passes the stored "Count this install" on to the
// requests to the metadata server (at start and after each change).
func (a *API) ApplyCountryCount(ctx context.Context) {
	if a.s.CountryCount != nil {
		a.s.CountryCount(a.uiSettings(ctx).CountInstall)
	}
}

func (a *API) getUISettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.uiSettings(r.Context()))
}

func (a *API) putUISettings(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AutoplayTrailers *bool `json:"autoplay_trailers"`
		KeepYTDLPUpdated *bool `json:"keep_ytdlp_updated"`
		SigninLibraryArt *bool `json:"signin_library_art"`
		ArtworkCacheGB   *int  `json:"artwork_cache_gb"`
		RoutineDays      *int  `json:"history_routine_days"`
		OtherDays        *int  `json:"history_other_days"`
		CountInstall     *bool `json:"count_install"`
	}
	if !decode(w, r, &in, `send {"autoplay_trailers": true, "keep_ytdlp_updated": true, "signin_library_art": true, "artwork_cache_gb": 5, "count_install": true}`) {
		return
	}
	if in.RoutineDays != nil || in.OtherDays != nil {
		// History is also the audit trail: only the owner sets how long it lasts.
		if who, signed := auth.From(r.Context()); !signed || who.Role != auth.RoleOwner {
			fail(w, http.StatusForbidden, "only the owner may change how long History is kept")
			return
		}
		k := store.ReadHistoryKeep(r.Context(), a.s.Store.Read())
		if in.RoutineDays != nil {
			k.RoutineDays = *in.RoutineDays
		}
		if in.OtherDays != nil {
			k.OtherDays = *in.OtherDays
		}
		if err := a.s.Store.SaveHistoryKeep(r.Context(), k, actor(r)); err != nil {
			if errors.Is(err, store.ErrHistoryKeep) {
				fail(w, http.StatusBadRequest, err.Error())
				return
			}
			internal(w, err)
			return
		}
	}
	if in.ArtworkCacheGB != nil {
		if !artworkCacheSizes[*in.ArtworkCacheGB] {
			fail(w, http.StatusBadRequest, "artwork_cache_gb is 1, 2, 5, 10 or 20")
			return
		}
		now := store.FormatTime(time.Now())
		by, _ := json.Marshal(actor(r))
		if err := a.s.Store.Write(r.Context(), func(tx *sql.Tx) error {
			_, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('this_app', ?, ?, ?, ?, ?, ?)
			ON CONFLICT (grp, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
				settingArtworkCache, strconv.Itoa(*in.ArtworkCacheGB), now, string(by), now, string(by))
			return err
		}); err != nil {
			internal(w, err)
			return
		}
		a.applyArtworkCap(r.Context())
	}
	for key, on := range map[string]*bool{settingAutoplay: in.AutoplayTrailers, SettingYTDLPUpdate: in.KeepYTDLPUpdated, settingSigninLibraryArt: in.SigninLibraryArt, settingCountryCount: in.CountInstall} {
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
	if in.CountInstall != nil {
		a.ApplyCountryCount(r.Context())
	}
	writeJSON(w, http.StatusOK, a.uiSettings(r.Context()))
}
