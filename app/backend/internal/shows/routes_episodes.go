package shows

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// EpisodeSlot is one episode in one Version.
type EpisodeSlot struct {
	ID         int64  `json:"id"`
	Version    string `json:"version"`
	VersionID  int64  `json:"version_id"`
	State      string `json:"state"`
	StateLabel string `json:"state_label"`
	Reason     string `json:"reason,omitempty"`
	FileID     *int64 `json:"file_id,omitempty"`
	// File is the episode's file (path, size, quality, media info), asked for with files=1.
	File *mediafiles.File `json:"file,omitempty"`
}

// EpisodeView is one episode with its Slots.
type EpisodeView struct {
	ID       int64      `json:"id"`
	Season   int        `json:"season"`
	Episode  int        `json:"episode"`
	Absolute *int       `json:"absolute,omitempty"`
	Name     string     `json:"name,omitempty"`
	AirDate  *time.Time `json:"air_date,omitempty"`
	AirDay   string     `json:"air_day,omitempty"`
	Runtime  *int       `json:"runtime,omitempty"`
	// RuntimeMinutes repeats Runtime under the name title views use.
	RuntimeMinutes *int              `json:"runtime_minutes,omitempty"`
	Still          string            `json:"still,omitempty"` // still picture URL (w780)
	KeepUpToDate   bool              `json:"keep_up_to_date"`
	Dropped        bool              `json:"dropped,omitempty"`
	IDs            map[string]string `json:"ids"`
	Slots          []EpisodeSlot     `json:"slots"`
	// OutsideOrder marks an episode the show's chosen episode order leaves
	// out; it keeps TMDB's numbers.
	OutsideOrder bool `json:"outside_order,omitempty"`
	// TMDBSeason and TMDBEpisode are TMDB's own numbers, set when a chosen
	// episode order numbers the episode differently.
	TMDBSeason  *int `json:"tmdb_season,omitempty"`
	TMDBEpisode *int `json:"tmdb_episode,omitempty"`
}

// EpisodeViewOf is an episode Item as the API shows it, without Slots.
func EpisodeViewOf(it mediafiles.Item) EpisodeView {
	v := EpisodeView{ID: it.ID, Absolute: it.Numbers.Absolute, AirDate: it.AirDate, Runtime: it.Runtime, RuntimeMinutes: it.Runtime, Still: it.Still, KeepUpToDate: it.KeepUpToDate,
		Dropped: it.DroppedAt != nil, IDs: it.SourceIDs, Slots: []EpisodeSlot{}}
	if it.Numbers.Season != nil {
		v.Season = *it.Numbers.Season
	}
	if it.Numbers.Episode != nil {
		v.Episode = *it.Numbers.Episode
	}
	if it.Name != nil {
		v.Name = *it.Name
	}
	if it.AirDay != nil {
		v.AirDay = *it.AirDay
	}
	if v.IDs == nil {
		v.IDs = map[string]string{}
	}
	return v
}

// byOrder puts an episode view on the show's chosen episode order: its
// numbers there, or the outside-order mark when the order leaves it out.
// Specials keep their own numbers.
func (v *EpisodeView) byOrder(it mediafiles.Item) {
	n := it.Numbers
	switch {
	case v.Season == 0:
	case n.AltSeason == nil || n.AltEpisode == nil || *n.AltSeason <= 0:
		v.OutsideOrder = true
	case *n.AltSeason != v.Season || *n.AltEpisode != v.Episode:
		s, e := v.Season, v.Episode
		v.TMDBSeason, v.TMDBEpisode = &s, &e
		v.Season, v.Episode = *n.AltSeason, *n.AltEpisode
	}
}

// EpisodeHandlers serves a show's episodes.
type EpisodeHandlers struct {
	Store *database.Store
	Lib   *library.Service
	// TitleID reads the {id} Title, answering 404 itself.
	TitleID func(w http.ResponseWriter, r *http.Request) (mediafiles.Title, bool)
	// Fail answers a show or library error.
	Fail func(w http.ResponseWriter, err error)
	// StateLabels names each Slot state.
	StateLabels map[mediafiles.SlotState]string
	// Trim leaves out what only people who manage the Library may see.
	Trim func(ctx context.Context, slots map[int64][]EpisodeSlot)
}

// Routes registers the episode routes.
func (h *EpisodeHandlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/titles/{id}/episodes", h.listEpisodes)
	m.HandleFunc("PUT /api/v1/episodes/{id}", h.setEpisode)
}

// listEpisodes lists a show's episodes in aired order, one season with
// ?season=<number>.
func (h *EpisodeHandlers) listEpisodes(w http.ResponseWriter, r *http.Request) {
	t, ok := ShowID(h.TitleID, w, r)
	if !ok {
		return
	}
	db := h.Store.Read()
	var season *int
	if sv := r.URL.Query().Get("season"); sv != "" {
		n, err := strconv.Atoi(sv)
		if err != nil || n < 0 {
			webserver.Fail(w, http.StatusBadRequest, "season must be a number")
			return
		}
		season = &n
	}
	grouped := t.EpisodeGroupID != nil && *t.EpisodeGroupID != ""
	list := mediafiles.ShowEpisodes
	if grouped {
		list = mediafiles.ShowEpisodesByOrder
	}
	its, err := list(r.Context(), db, t.ID, season)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	vs, err := mediafiles.VersionsOfTitle(r.Context(), db, t.ID)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	label := map[int64]string{}
	for _, v := range vs {
		label[v.ID] = v.Label
	}
	slots, err := mediafiles.EpisodeSlotsOfTitle(r.Context(), db, t.ID)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	// files=1 adds each episode's file, for the episode details on the title page.
	files := map[int64]*mediafiles.File{}
	if r.URL.Query().Get("files") == "1" {
		fs, err := mediafiles.EpisodeFilesOfTitle(r.Context(), db, t.ID)
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		for i := range fs {
			files[fs[i].ID] = &fs[i]
		}
	}
	byItem := map[int64][]EpisodeSlot{}
	now := time.Now()
	for _, s := range slots {
		es := EpisodeSlot{ID: s.ID, Version: label[s.VersionID], VersionID: s.VersionID, State: string(s.State), StateLabel: h.StateLabels[s.State], FileID: s.FileID}
		if s.FileID != nil {
			es.File = files[*s.FileID]
		}
		if s.LastReason != nil {
			es.Reason = *s.LastReason
		}
		if sources.StuckChoice(s, now) {
			es.Reason = sources.StuckChoiceReason
		}
		byItem[s.ItemID] = append(byItem[s.ItemID], es)
	}
	h.Trim(r.Context(), byItem)
	out := make([]EpisodeView, 0, len(its))
	for _, it := range its {
		v := EpisodeViewOf(it)
		if grouped {
			v.byOrder(it)
		}
		if sl := byItem[it.ID]; sl != nil {
			v.Slots = sl
		}
		out = append(out, v)
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"episodes": out, "episode_order": t.EpisodeOrder})
}

func (h *EpisodeHandlers) setEpisode(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		webserver.Fail(w, http.StatusNotFound, "not found")
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
	it, err := New(h.Lib).SetEpisode(r.Context(), id, *body.KeepUpToDate, actor(r))
	if errors.Is(err, database.ErrNotFound) {
		webserver.Fail(w, http.StatusNotFound, "no such episode")
		return
	}
	if err != nil {
		h.Fail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, EpisodeViewOf(it))
}
