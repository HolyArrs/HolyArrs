package sources

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/store"
)

// Search routes: the Releases a search found with their verdicts, choosing
// one by hand, turning one down, the Held and Turned down lists, bulk
// searches and the search settings.

// Reason is one line of a verdict in plain words.
type Reason struct {
	Code   string `json:"code"`   // step.rule, stable for the screens
	Effect string `json:"effect"` // pass, fail, add, subtract, warn, hold, rank
	Text   string `json:"text"`
}

// ReleaseView is a stored Release with its verdict and reasons.
type ReleaseView struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	Seeders    *int   `json:"seeders,omitempty"`
	Peers      *int   `json:"peers,omitempty"`
	Protocol   string `json:"protocol"`
	State      string `json:"state"`
	FoundBy    string `json:"found_by"`
	JobID      *int64 `json:"job_id,omitempty"`
	SourceID   int64  `json:"source_id"`
	SourceName string `json:"source_name"`
	SlotID     *int64 `json:"slot_id,omitempty"`
	Version    string `json:"version,omitempty"`
	TitleID    *int64 `json:"title_id,omitempty"`
	TitleName  string `json:"title_name,omitempty"`
	// Shows: the episodes the Release holds or was picked for ("S01E02",
	// "S01 (10 episodes)"), their Slots, and the episodes it was turned
	// down for one by one.
	Episode       string  `json:"episode,omitempty"`
	Album         string  `json:"album,omitempty"` // artists: the album the Release is for
	SlotIDs       []int64 `json:"slot_ids,omitempty"`
	TurnedDownFor string  `json:"turned_down_for,omitempty"`
	turnedSlots   []int64
	Published     *time.Time     `json:"published_at,omitempty"`
	HoldUntil     *time.Time     `json:"hold_until,omitempty"`
	Outcome       string         `json:"outcome"`
	Passed        bool           `json:"passed"`
	Rank          int            `json:"rank"`
	Points        int            `json:"points"`
	Step          string         `json:"step,omitempty"`
	GoodEnough    bool           `json:"good_enough"`
	NeedsForce    bool           `json:"needs_force"`
	GateSkip      bool           `json:"gate_skipped"`
	Reasons       []Reason       `json:"reasons"`
	Verdict       map[string]any `json:"verdict"`
	CreatedAt     time.Time      `json:"created_at"`
}

func releaseView(x domain.Release) ReleaseView {
	v := ReleaseView{ID: x.ID, Name: x.Name, Size: x.Size, Seeders: x.Seeders, Peers: x.Peers, Protocol: x.Protocol, State: x.State, FoundBy: x.FoundBy,
		JobID: x.JobID, SourceID: x.SourceID, Published: x.PublishedAt, HoldUntil: x.HoldUntil, Verdict: x.Verdict, CreatedAt: x.CreatedAt,
		Reasons: []Reason{}, NeedsForce: x.State == "refused" || x.State == "turned_down" || RefusedByRules(x)}
	if len(x.MatchedSlotIDs) > 0 {
		id := x.MatchedSlotIDs[0]
		v.SlotID = &id
	}
	if many := PickSlots(x); len(many) > 1 {
		v.SlotIDs = many
	} else if len(x.MatchedSlotIDs) > 1 {
		v.SlotIDs = x.MatchedSlotIDs
	}
	if only := TurnedDownSlots(x); len(only) > 0 {
		v.turnedSlots = only
		if x.State == "turned_down" {
			id := only[0]
			v.SlotID = &id
		}
	}
	v.Outcome, _ = x.Verdict["outcome"].(string)
	v.Passed, _ = x.Verdict["passed"].(bool)
	v.GoodEnough, _ = x.Verdict["good_enough"].(bool)
	if n, ok := x.Verdict["rank"].(float64); ok {
		v.Rank = int(n)
	}
	if n, ok := x.Verdict["points"].(float64); ok {
		v.Points = int(n)
	}
	if st, ok := x.Verdict["step"].(map[string]any); ok {
		v.Step, _ = st["name"].(string)
	}
	rs, _ := x.Verdict["reasons"].([]any)
	for _, y := range rs {
		m, _ := y.(map[string]any)
		step, _ := m["step"].(string)
		rule, _ := m["rule"].(string)
		eff, _ := m["effect"].(string)
		text, _ := m["detail"].(string)
		if pts, ok := m["points"].(float64); ok && pts != 0 {
			text = fmt.Sprintf("%s (%+d points)", text, int(pts))
		}
		if step+"."+rule == "hard.gate_skipped" {
			v.GateSkip = true
		}
		v.Reasons = append(v.Reasons, Reason{Code: step + "." + rule, Effect: eff, Text: text})
	}
	if why, _ := x.Verdict["turned_down_reason"].(string); why != "" {
		v.Reasons = append(v.Reasons, Reason{Code: "turned_down.reason", Effect: "fail", Text: "turned down: " + why})
	}
	return v
}

// named fills in Source, Title and Version names for a page of Releases.
func (h *Handlers) named(ctx context.Context, vs []ReleaseView) {
	db := h.Store.Read()
	for i := range vs {
		vs[i].SourceName = sourceName(ctx, db, vs[i].SourceID)
		if vs[i].SlotID != nil {
			if tid, title, version, err := store.SlotNames(ctx, db, *vs[i].SlotID); err == nil {
				vs[i].TitleID, vs[i].TitleName, vs[i].Version = &tid, title, version
			}
			slots := vs[i].SlotIDs
			if len(slots) == 0 {
				slots = []int64{*vs[i].SlotID}
			}
			vs[i].Episode = h.Catalog.EpisodeLabels(ctx, slots)
			vs[i].Album = h.Catalog.AlbumLabels(ctx, slots)
			vs[i].TurnedDownFor = h.Catalog.EpisodeLabels(ctx, vs[i].turnedSlots)
		}
	}
}

// titleReleases lists the Releases of the newest search of each Slot of
// the Title (or of slot_id), winners first, then passed over, then
// refused; paged with limit and offset.
func (h *Handlers) titleReleases(w http.ResponseWriter, r *http.Request) {
	t, ok := h.Catalog.TitleFromPath(w, r)
	if !ok {
		return
	}
	n, off := queryPage(r, 100)
	db := h.Store.Read()
	var filters []slotFilter
	for _, f := range []struct{ param, cond string }{{"slot_id", " AND s.id = ?"}, {"item_id", " AND i.id = ?"},
		{"season", " AND json_extract(i.numbers, '$.season') = ?"}, {"version_id", " AND s.version_id = ?"}, {"album_id", " AND i.part_id = ?"}} {
		sid := r.URL.Query().Get(f.param)
		if sid == "" {
			continue
		}
		id, err := strconv.ParseInt(sid, 10, 64)
		if err != nil {
			webserver.Fail(w, http.StatusBadRequest, f.param+" must be a number")
			return
		}
		filters = append(filters, slotFilter{cond: f.cond, id: id})
	}
	jobIDs, err := latestSearchJobs(r.Context(), db, t.ID, filters)
	if err != nil {
		webserver.Internal(w, "sources", err)
		return
	}
	out := []ReleaseView{}
	total := 0
	if len(jobIDs) > 0 {
		var rels []domain.Release
		rels, total, err = releasesOfJobs(r.Context(), db, jobIDs, n, off)
		if err != nil {
			webserver.Internal(w, "sources", err)
			return
		}
		for _, x := range rels {
			out = append(out, releaseView(x))
		}
		h.named(r.Context(), out)
	}
	// Searches of this Title still running, so a screen can wait for them.
	running := runningSearchJobs(r.Context(), db, t.ID)
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"releases": out, "total": total, "limit": n, "offset": off, "running_job_ids": running})
}

func (h *Handlers) searcher() *Searcher { return h.search() }

// SearchError answers a search refusal with its status and reason.
func SearchError(w http.ResponseWriter, err error) {
	var nf *NeedsForceError
	switch {
	case errors.As(err, &nf):
		webserver.WriteJSON(w, http.StatusConflict, map[string]any{"error": nf.Error(), "needs_force": true, "reasons": nf.Reasons})
	case errors.Is(err, ErrNoRelease):
		webserver.Fail(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrState):
		webserver.Fail(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrNoSlot):
		webserver.Fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrBusy):
		webserver.Fail(w, http.StatusConflict, err.Error())
	default:
		webserver.Internal(w, "sources", err)
	}
}

// chooseRelease sends a Release a person picked. Body (optional):
// {"force": bool, "slot_id": int}.
func (h *Handlers) chooseRelease(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Force  bool  `json:"force"`
		SlotID int64 `json:"slot_id"`
	}
	if r.ContentLength != 0 && !webserver.Decode(w, r, &in, `body is {"force": bool, "slot_id": int}, both optional`) {
		return
	}
	ch, err := h.search().Choose(r.Context(), id, in.SlotID, in.Force, actorOf(r))
	if err != nil {
		SearchError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, ch)
}

// turnDownRelease puts a Release on the Turned down list. Body (optional):
// {"reason": text}.
func (h *Handlers) turnDownRelease(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Reason string `json:"reason"`
		SlotID int64  `json:"slot_id"`
	}
	if r.ContentLength != 0 && !webserver.Decode(w, r, &in, `body is {"reason": text, "slot_id": int}, both optional`) {
		return
	}
	if len(in.Reason) > 500 {
		webserver.Fail(w, http.StatusBadRequest, "reason is at most 500 characters")
		return
	}
	if err := h.search().TurnDownFor(r.Context(), id, in.SlotID, strings.TrimSpace(in.Reason), actorOf(r)); err != nil {
		SearchError(w, err)
		return
	}
	var rel domain.Release
	if err := database.Get(r.Context(), h.Store.Read(), "releases", id, &rel); err != nil {
		webserver.Internal(w, "sources", err)
		return
	}
	v := []ReleaseView{releaseView(rel)}
	h.named(r.Context(), v)
	webserver.WriteJSON(w, http.StatusOK, v[0])
}

func (h *Handlers) listState(state string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		n, off := queryPage(r, 50)
		list, total, err := h.search().List(r.Context(), state, n, off)
		if err != nil {
			webserver.Internal(w, "sources", err)
			return
		}
		out := make([]ReleaseView, 0, len(list))
		for _, x := range list {
			out = append(out, releaseView(x.Release))
		}
		h.named(r.Context(), out)
		webserver.WriteJSON(w, http.StatusOK, map[string]any{"releases": out, "total": total, "limit": n, "offset": off})
	}
}

func (h *Handlers) removeFrom(state string, one bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var id int64
		if one {
			var ok bool
			if id, ok = pathID(w, r); !ok {
				return
			}
		}
		var n int
		var err error
		if state == "held" {
			n, err = h.search().RemoveHeld(r.Context(), id, actorOf(r))
		} else {
			n, err = h.search().RemoveTurnedDown(r.Context(), id, actorOf(r))
		}
		if err != nil {
			SearchError(w, err)
			return
		}
		webserver.WriteJSON(w, http.StatusOK, map[string]any{"removed": n})
	}
}

// bulkSearch queues one search_missing Job for the whole Missing list or
// the whole Better copy list; {"kind": "movie", "show" or "artist"} keeps it
// to that kind.
func (h *Handlers) bulkSearch(which string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Kind string `json:"kind"`
		}
		if r.ContentLength != 0 && !webserver.Decode(w, r, &in, `send nothing for everything, or {"kind": "movie", "show" or "artist"}`) {
			return
		}
		params, key := map[string]any{"which": which}, "search_missing:"+which
		switch in.Kind {
		case "":
		case "movie", "show", "artist":
			params["kind"], key = in.Kind, key+":"+in.Kind
		default:
			webserver.Fail(w, http.StatusBadRequest, `kind must be "movie", "show" or "artist"`)
			return
		}
		id, _, err := h.Queue.Enqueue(r.Context(), jobs.KindSearchMissing, nil, params, key, actorOf(r))
		if err != nil {
			webserver.Internal(w, "sources", err)
			return
		}
		webserver.WriteJSON(w, http.StatusAccepted, map[string]any{"job_ids": []int64{id}})
	}
}

func (h *Handlers) feedCheckNow(w http.ResponseWriter, r *http.Request) {
	id, _, err := h.Queue.Enqueue(r.Context(), jobs.KindFeedCheck, nil, map[string]any{}, "schedule:"+jobs.KindFeedCheck, actorOf(r))
	if err != nil {
		webserver.Internal(w, "sources", err)
		return
	}
	webserver.WriteJSON(w, http.StatusAccepted, map[string]any{"job_ids": []int64{id}})
}

// SearchSettings are the search choices a person can change.
type SearchSettings struct {
	FeedCheckMinutes    int  `json:"feed_check_minutes"`    // 0 is off, else 10 to 120
	MissingSearchCount  int  `json:"missing_search_count"`  // per daily run; 0 is off, at most 500
	MissingSearchBetter bool `json:"missing_search_better"` // the daily run also looks for better copies
	TryNextDownloader   bool `json:"try_next_downloader"`
	// Shows.
	MissingSearchShows bool   `json:"missing_search_shows"` // the daily and bulk searches cover episodes
	SeasonPackUpgrade  string `json:"season_pack_upgrade"`  // all: every episode a pack holds must gain; any: one is enough
	SeasonPackPrefer   string `json:"season_pack_prefer"`   // rank: a pack wins ties; always: a passing pack wins
	MultiSeasonPacks   bool   `json:"multi_season_packs"`   // Releases with several seasons may be taken
	// Music.
	MissingSearchMusic    bool `json:"missing_search_music"`    // the daily and bulk searches cover albums
	MusicDiscographyPacks bool `json:"music_discography_packs"` // a whole-discography Release may fill an album
	AlbumTrackTolerance   int  `json:"album_track_tolerance"`   // files a Release may lack against the album's tracks; -1 off
	AlbumExtraFiles       int  `json:"album_extra_files"`       // files a bare file count is taken to hold besides tracks; 0 to 20
	ArtistSearchCap       int  `json:"artist_search_cap"`       // most album searches one Search now on an artist queues; 1 to 2000
	// Early and fake Releases.
	RefuseEarlyReleases    bool `json:"refuse_early_releases"`     // a Release posted before its movie, episode or album came out is refused
	EarlyReleaseGraceHours int  `json:"early_release_grace_hours"` // 0 to 720
	RefuseTinyReleases     bool `json:"refuse_tiny_releases"`      // far too small for what it holds, or an empty or old-container file at import
	ImportMinVideoMB       int  `json:"import_min_video_mb"`       // least video file placed; 0 uses the fixed floor (1 MB a minute, 20 MB unknown length)
	ImportMinTrackKB       int  `json:"import_min_track_kb"`       // least track file placed; 0 uses the fixed floor (512 KB)
	ImportSizeFloors       bool `json:"import_size_floors"`        // the fixed least sizes at import and the judged-size check; off leaves empty files and the sizes set above
	HoldUnknownReleases    bool `json:"hold_unknown_releases"`     // automatic searches hold a Release of unknown size or dates for a person; off warns only
	EpisodeDefaultMinutes  int  `json:"episode_default_minutes"`   // length taken for an episode of unknown length by the size floor; 1 to 240
}

func (h *Handlers) readSearchSettings(ctx context.Context) (SearchSettings, error) {
	db := h.Store.Read()
	var st SearchSettings
	every, enabled, err := jobs.ScheduleEvery(ctx, db, jobs.KindFeedCheck)
	if err != nil {
		return st, err
	}
	if enabled {
		st.FeedCheckMinutes = every / 60
	}
	missingOn := jobs.ScheduleEnabled(ctx, db, jobs.KindSearchMissing)
	st.MissingSearchCount, _ = strconv.Atoi(Setting(ctx, db, SettingMissingCount))
	if !missingOn {
		st.MissingSearchCount = 0
	}
	st.MissingSearchBetter = Setting(ctx, db, SettingMissingBetter) == "true"
	st.TryNextDownloader = Setting(ctx, db, "try_next_downloader") != "false"
	st.MissingSearchShows = Setting(ctx, db, SettingMissingShows) != "false"
	st.SeasonPackUpgrade = "all"
	if Setting(ctx, db, SettingPackUpgrade) == "any" {
		st.SeasonPackUpgrade = "any"
	}
	st.SeasonPackPrefer = "rank"
	if Setting(ctx, db, SettingPackPrefer) == "always" {
		st.SeasonPackPrefer = "always"
	}
	st.MultiSeasonPacks = Setting(ctx, db, SettingMultiSeason) == "true"
	st.MissingSearchMusic = Setting(ctx, db, SettingMissingMusic) != "false"
	st.MusicDiscographyPacks = Setting(ctx, db, SettingDiscography) == "true"
	if n, err := strconv.Atoi(Setting(ctx, db, SettingTrackTolerance)); err == nil {
		st.AlbumTrackTolerance = n
	}
	st.AlbumExtraFiles, _ = strconv.Atoi(Setting(ctx, db, SettingExtraFiles))
	st.ArtistSearchCap = ArtistSearchCap(ctx, db)
	ch := Checks(ctx, db)
	st.RefuseEarlyReleases, st.EarlyReleaseGraceHours, st.RefuseTinyReleases = ch.Early, int(ch.Grace.Hours()), ch.Tiny
	st.ImportMinVideoMB, _ = strconv.Atoi(Setting(ctx, db, "import_min_video_mb"))
	st.ImportMinTrackKB, _ = strconv.Atoi(Setting(ctx, db, "import_min_track_kb"))
	st.ImportSizeFloors = Setting(ctx, db, SettingImportFloors) != "false"
	st.HoldUnknownReleases, st.EpisodeDefaultMinutes = ch.HoldUnknown, ch.EpisodeMinutes
	return st, nil
}

func (h *Handlers) getSearchSettings(w http.ResponseWriter, r *http.Request) {
	st, err := h.readSearchSettings(r.Context())
	if err != nil {
		webserver.Internal(w, "sources", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, st)
}

func (h *Handlers) putSearchSettings(w http.ResponseWriter, r *http.Request) {
	// Fields left out keep their value.
	in, err := h.readSearchSettings(r.Context())
	if err != nil {
		webserver.Internal(w, "sources", err)
		return
	}
	if !webserver.Decode(w, r, &in, "body is the search settings") {
		return
	}
	if in.SeasonPackUpgrade != "all" && in.SeasonPackUpgrade != "any" {
		webserver.Fail(w, http.StatusBadRequest, "season_pack_upgrade is all or any")
		return
	}
	if in.SeasonPackPrefer != "rank" && in.SeasonPackPrefer != "always" {
		webserver.Fail(w, http.StatusBadRequest, "season_pack_prefer is rank or always")
		return
	}
	if in.AlbumTrackTolerance < -1 || in.AlbumTrackTolerance > 50 {
		webserver.Fail(w, http.StatusBadRequest, "album_track_tolerance is -1 (off) or 0 to 50")
		return
	}
	if in.AlbumExtraFiles < 0 || in.AlbumExtraFiles > 20 {
		webserver.Fail(w, http.StatusBadRequest, "album_extra_files is 0 to 20")
		return
	}
	if in.ArtistSearchCap < 1 || in.ArtistSearchCap > 2000 {
		webserver.Fail(w, http.StatusBadRequest, "artist_search_cap is 1 to 2000")
		return
	}
	if in.EarlyReleaseGraceHours < 0 || in.EarlyReleaseGraceHours > 720 {
		webserver.Fail(w, http.StatusBadRequest, "early_release_grace_hours is 0 to 720")
		return
	}
	if in.ImportMinVideoMB < 0 || in.ImportMinVideoMB > 10000 || in.ImportMinTrackKB < 0 || in.ImportMinTrackKB > 100000 {
		webserver.Fail(w, http.StatusBadRequest, "import_min_video_mb is 0 to 10000 and import_min_track_kb 0 to 100000")
		return
	}
	if in.EpisodeDefaultMinutes < 1 || in.EpisodeDefaultMinutes > 240 {
		webserver.Fail(w, http.StatusBadRequest, "episode_default_minutes is 1 to 240")
		return
	}
	if in.FeedCheckMinutes != 0 && (in.FeedCheckMinutes < 10 || in.FeedCheckMinutes > 120) {
		webserver.Fail(w, http.StatusBadRequest, "feed_check_minutes is 0 (off) or 10 to 120")
		return
	}
	if in.MissingSearchCount < 0 || in.MissingSearchCount > 500 {
		webserver.Fail(w, http.StatusBadRequest, "missing_search_count is 0 (off) to 500")
		return
	}
	err = h.Store.Write(r.Context(), func(tx *sql.Tx) error { return saveSearchSettings(tx, in, actorOf(r), time.Now()) })
	if err != nil {
		webserver.Internal(w, "sources", err)
		return
	}
	// The change is logged once by logSettingsChanges as an audit Event (area
	// and who, never the values), so it is not written or sent live here.
	h.getSearchSettings(w, r)
}

// SearchView is one search Job for Home: what it looks for, how far it is
// and, once done, what it found.
type SearchView struct {
	JobID     int64      `json:"job_id"`
	Kind      string     `json:"kind"` // search_slot, search_missing, feed_check
	State     string     `json:"state"`
	ByHand    bool       `json:"by_hand"`
	OnlyShow  bool       `json:"only_show"` // a Choose a release search: judged and stored, nothing picked
	TitleID   *int64     `json:"title_id,omitempty"`
	TitleName string     `json:"title_name,omitempty"`
	Version   string     `json:"version,omitempty"`
	Episode   string     `json:"episode,omitempty"` // shows: "S01E02", or "S01" for a season search
	Album     string     `json:"album,omitempty"`   // artists: the album searched
	Progress  float64    `json:"progress"`
	Note      string     `json:"note,omitempty"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
	Found     int        `json:"found"`
	Refused   int        `json:"refused"`
	Outcome   string     `json:"outcome,omitempty"` // chosen, held, shown, none, failed, episodes (searched one by one)
	Picked    string     `json:"picked,omitempty"`  // the chosen or Held Release name
	Error     string     `json:"error,omitempty"`
}

// searches lists the search Jobs queued or running, oldest first, and the
// newest finished ones (limit, default 10) with what each found.
func (h *Handlers) searches(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	db := h.Store.Read()
	running, recent, err := searchJobs(ctx, db, queryLimit(r, 10))
	if err != nil {
		webserver.Internal(w, "sources", err)
		return
	}
	view := func(js []domain.Job) []SearchView {
		out := make([]SearchView, 0, len(js))
		for _, j := range js {
			v := SearchView{JobID: j.ID, Kind: j.Kind, State: j.State, Progress: j.Progress, StartedAt: j.StartedAt, EndedAt: j.EndedAt}
			v.ByHand, _ = j.Params["by_hand"].(bool)
			if c, ok := j.Params["choose"].(bool); ok && !c {
				v.OnlyShow = true
			}
			if j.ProgressNote != nil {
				v.Note = *j.ProgressNote
			}
			if j.Error != nil {
				v.Error = j.Error.Message
			}
			if j.Subject != nil && j.Subject.Type == "slot" {
				if tid, title, version, err := store.SlotNames(ctx, db, j.Subject.ID); err == nil {
					v.TitleID, v.TitleName, v.Version = &tid, title, version
				}
				v.Episode = h.Catalog.EpisodeLabels(ctx, []int64{j.Subject.ID})
				v.Album = h.Catalog.AlbumLabels(ctx, []int64{j.Subject.ID})
			}
			if j.Subject != nil && j.Subject.Type == "part" {
				tid, name, season, album := h.Catalog.PartLabel(ctx, j.Subject.ID)
				if tid > 0 {
					v.TitleID, v.TitleName, v.Episode, v.Album = &tid, name, season, album
				}
				if vid, ok := j.Params["version_id"].(float64); ok {
					v.Version = store.VersionLabel(ctx, db, int64(vid))
				}
			}
			if n, ok := j.Result["releases"].(float64); ok {
				v.Found = int(n)
			}
			if n, ok := j.Result["refused"].(float64); ok {
				v.Refused = int(n)
			}
			switch {
			case j.State == "failed":
				v.Outcome = "failed"
			case j.Result["chosen_release_id"] != nil:
				v.Outcome = "chosen"
			case j.Result["held_release_id"] != nil:
				v.Outcome = "held"
			case j.Result["shown"] == true:
				v.Outcome = "shown"
			case j.Result["episodes_queued"] != nil:
				v.Outcome = "episodes"
			case j.Result["album_search_job_id"] != nil:
				v.Outcome = "album"
			case j.State == "done" && (j.Kind == jobs.KindSearchSlot || j.Kind == jobs.KindSearchSeason || j.Kind == jobs.KindSearchAlbum):
				v.Outcome = "none"
			}
			for _, k := range []string{"chosen_release_id", "held_release_id"} {
				if id, ok := j.Result[k].(float64); ok {
					v.Picked = releaseName(ctx, db, int64(id))
				}
			}
			out = append(out, v)
		}
		return out
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"running": view(running), "recent": view(recent)})
}
