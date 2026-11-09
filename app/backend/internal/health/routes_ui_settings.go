package health

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// App-wide look settings everyone's browser reads: today only whether
// trailers play by themselves (heroes and hover cards). Stored in the
// this_app settings group.

const (
	settingAutoplay = "autoplay_trailers"
	// settingCountryCount is "Count this install" (the metadata server's
	// per-country install total).
	settingCountryCount = SettingCountryCount
	// SettingSigninLibraryArt is "Show Library art on the sign-in page".
	SettingSigninLibraryArt = "signin_library_art"
)

// UIHandlers serves GET and PUT /settings/ui. SetArtworkCap (may be nil)
// sets the picture cache's cap in bytes; CountryCount (may be nil) passes
// "Count this install" on to the metadata server's requests.
type UIHandlers struct {
	Store         *database.Store
	Actor         func(*http.Request) domain.Actor
	SetArtworkCap func(int64)
	CountryCount  func(bool)
}

// Routes mounts the UI settings routes.
func (h *UIHandlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/settings/ui", h.get)
	m.HandleFunc("PUT /api/v1/settings/ui", h.put)
}

// UISettings is the answer of GET /settings/ui.
type UISettings struct {
	// AutoplayTrailers lets heroes and hover cards play a muted trailer by
	// themselves (never for Kid accounts or with Reduce motion). Default on.
	AutoplayTrailers bool `json:"autoplay_trailers"`
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

// ApplyArtworkCap sets the picture cache's cap from Settings.
func (h *UIHandlers) ApplyArtworkCap(ctx context.Context) {
	if h.SetArtworkCap != nil {
		h.SetArtworkCap(int64(UISettingsOf(ctx, h.Store).ArtworkCacheGB) << 30)
	}
}

// UISettingsOf reads the UI settings.
func UISettingsOf(ctx context.Context, st *database.Store) UISettings {
	v, _ := database.Setting(ctx, st.Read(), "this_app", settingAutoplay)
	l, _ := database.Setting(ctx, st.Read(), "this_app", SettingSigninLibraryArt)
	g, _ := database.Setting(ctx, st.Read(), "this_app", settingArtworkCache)
	gb, err := strconv.Atoi(g)
	if err != nil || !artworkCacheSizes[gb] {
		gb = int(library.DefaultArtworkCap >> 30)
	}
	k := database.ReadHistoryKeep(ctx, st.Read())
	return UISettings{AutoplayTrailers: v != "off", SigninLibraryArt: l == "on", ArtworkCacheGB: gb,
		HistoryRoutineDays: k.RoutineDays, HistoryOtherDays: k.OtherDays, CountInstall: CountInstall(ctx, st.Read())}
}

// ApplyCountryCount passes the stored "Count this install" on to the
// requests to the metadata server (at start and after each change).
func (h *UIHandlers) ApplyCountryCount(ctx context.Context) {
	if h.CountryCount != nil {
		h.CountryCount(UISettingsOf(ctx, h.Store).CountInstall)
	}
}

func (h *UIHandlers) get(w http.ResponseWriter, r *http.Request) {
	webserver.WriteJSON(w, http.StatusOK, UISettingsOf(r.Context(), h.Store))
}

func (h *UIHandlers) put(w http.ResponseWriter, r *http.Request) {
	// keep_ytdlp_updated, the old trailer tool's update switch, is not read:
	// older pages may still send it, and like any field not listed here it
	// is ignored.
	var in struct {
		AutoplayTrailers *bool `json:"autoplay_trailers"`
		SigninLibraryArt *bool `json:"signin_library_art"`
		ArtworkCacheGB   *int  `json:"artwork_cache_gb"`
		RoutineDays      *int  `json:"history_routine_days"`
		OtherDays        *int  `json:"history_other_days"`
		CountInstall     *bool `json:"count_install"`
	}
	if !webserver.Decode(w, r, &in, `send {"autoplay_trailers": true, "signin_library_art": true, "artwork_cache_gb": 5, "count_install": true}`) {
		return
	}
	if in.RoutineDays != nil || in.OtherDays != nil {
		// History is also the audit trail: only the owner sets how long it lasts.
		if who, signed := webserver.From(r.Context()); !signed || who.Role != webserver.RoleOwner {
			webserver.Fail(w, http.StatusForbidden, "only the owner may change how long History is kept")
			return
		}
		k := database.ReadHistoryKeep(r.Context(), h.Store.Read())
		if in.RoutineDays != nil {
			k.RoutineDays = *in.RoutineDays
		}
		if in.OtherDays != nil {
			k.OtherDays = *in.OtherDays
		}
		if err := h.Store.SaveHistoryKeep(r.Context(), k, h.Actor(r)); err != nil {
			if errors.Is(err, database.ErrHistoryKeep) {
				webserver.Fail(w, http.StatusBadRequest, err.Error())
				return
			}
			webserver.Internal(w, "api", err)
			return
		}
	}
	if in.ArtworkCacheGB != nil {
		if !artworkCacheSizes[*in.ArtworkCacheGB] {
			webserver.Fail(w, http.StatusBadRequest, "artwork_cache_gb is 1, 2, 5, 10 or 20")
			return
		}
		if err := h.Store.Write(r.Context(), func(tx *sql.Tx) error {
			return database.PutSetting(tx, "this_app", settingArtworkCache, strconv.Itoa(*in.ArtworkCacheGB), h.Actor(r), time.Now())
		}); err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		h.ApplyArtworkCap(r.Context())
	}
	for key, on := range map[string]*bool{settingAutoplay: in.AutoplayTrailers, SettingSigninLibraryArt: in.SigninLibraryArt, settingCountryCount: in.CountInstall} {
		if on == nil {
			continue
		}
		v := map[bool]string{true: "on", false: "off"}[*on]
		err := h.Store.Write(r.Context(), func(tx *sql.Tx) error {
			return database.PutSetting(tx, "this_app", key, v, h.Actor(r), time.Now())
		})
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
	}
	if in.CountInstall != nil {
		h.ApplyCountryCount(r.Context())
	}
	webserver.WriteJSON(w, http.StatusOK, UISettingsOf(r.Context(), h.Store))
}

// AutoplayOn reads "Autoplay trailers" from Settings.
func AutoplayOn(ctx context.Context, st *database.Store) bool {
	return UISettingsOf(ctx, st).AutoplayTrailers
}

// SigninLibraryArtOn reads "Show Library art on the sign-in page".
func SigninLibraryArtOn(ctx context.Context, st *database.Store) bool {
	return UISettingsOf(ctx, st).SigninLibraryArt
}
