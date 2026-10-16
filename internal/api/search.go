package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/source"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// Search routes (stage 3.1): searching a Title, the releases a search found
// with their verdicts, choosing one by hand, turning one down, the Held and
// Turned down lists, bulk searches and the search settings.

// page reads limit (1 to 500, default def) and offset (0 up) from the query.
func page(r *http.Request, def int) (int, int) {
	off, err := strconv.Atoi(r.URL.Query().Get("offset"))
	if err != nil || off < 0 {
		off = 0
	}
	return limit(r, def), off
}

// searchTitle queues a search_slot Job for each Slot of the Title that is
// looking (or the one Slot asked for). Body (optional): {"choose": false}
// only judges and stores the Releases for a person to choose from;
// "slot_id" searches one Slot. A search from here is a single-Item hand
// search: the protocol wait is skipped, and the Release gate too, with a
// warning in each verdict.
func (a *API) searchTitle(w http.ResponseWriter, r *http.Request) {
	t, ok := a.titleID(w, r)
	if !ok {
		return
	}
	var in showSearchIn
	if r.ContentLength != 0 && !decode(w, r, &in, `body is {"choose": bool, "slot_id": int}, both optional; for a show also "item_id", "season" and "version_id"; for an artist "album_id", "item_id" and "version_id"`) {
		return
	}
	if t.Kind == "show" {
		a.searchShow(w, r, t, in)
		return
	}
	if t.Kind == "artist" {
		a.searchArtist(w, r, t, in)
		return
	}
	if in.ItemID != nil || in.Season != nil || in.VersionID != nil || in.AlbumID != nil {
		fail(w, http.StatusBadRequest, "item_id, season, album_id and version_id are for shows and artists")
		return
	}
	slots, err := store.Select[domain.Slot](r.Context(), a.s.Store.Read(), "slots",
		"item_id IN (SELECT id FROM items WHERE title_id = ?) ORDER BY id", t.ID)
	if err != nil {
		internal(w, err)
		return
	}
	params := map[string]any{"by_hand": true}
	if in.Choose != nil && !*in.Choose {
		params["choose"] = false
	}
	ids := []int64{}
	for _, s := range slots {
		if in.SlotID != nil && s.ID != *in.SlotID {
			continue
		}
		// Its own key: a bulk search waiting for the same Slot must not
		// swallow a person's search (which skips the wait and the gate).
		key := "hand_search_slot:" + itoa(s.ID)
		if params["choose"] == false {
			key = "show_slot:" + itoa(s.ID)
		}
		id, _, err := a.s.Queue.Enqueue(r.Context(), source.KindSearchSlot, &domain.Subject{Type: "slot", ID: s.ID}, params, key, actor(r))
		if err != nil {
			internal(w, err)
			return
		}
		ids = append(ids, id)
	}
	if in.SlotID != nil && len(ids) == 0 {
		fail(w, http.StatusNotFound, "no such Slot on this Title")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_ids": ids})
}

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
		Reasons: []Reason{}, NeedsForce: x.State == "refused" || x.State == "turned_down"}
	if len(x.MatchedSlotIDs) > 0 {
		id := x.MatchedSlotIDs[0]
		v.SlotID = &id
	}
	if many := source.PickSlots(x); len(many) > 1 {
		v.SlotIDs = many
	} else if len(x.MatchedSlotIDs) > 1 {
		v.SlotIDs = x.MatchedSlotIDs
	}
	if only := source.TurnedDownSlots(x); len(only) > 0 {
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
func (a *API) named(ctx context.Context, vs []ReleaseView) {
	db := a.s.Store.Read()
	for i := range vs {
		_ = db.QueryRowContext(ctx, "SELECT name FROM sources WHERE id = ?", vs[i].SourceID).Scan(&vs[i].SourceName)
		if vs[i].SlotID != nil {
			var tid int64
			if db.QueryRowContext(ctx, `SELECT t.id, t.name, v.label FROM slots s JOIN items i ON i.id = s.item_id JOIN titles t ON t.id = i.title_id
				JOIN versions v ON v.id = s.version_id WHERE s.id = ?`, *vs[i].SlotID).Scan(&tid, &vs[i].TitleName, &vs[i].Version) == nil {
				vs[i].TitleID = &tid
			}
			slots := vs[i].SlotIDs
			if len(slots) == 0 {
				slots = []int64{*vs[i].SlotID}
			}
			vs[i].Episode = a.episodeLabels(ctx, slots)
			vs[i].Album = a.albumLabels(ctx, slots)
			vs[i].TurnedDownFor = a.episodeLabels(ctx, vs[i].turnedSlots)
		}
	}
}

// titleReleases lists the Releases of the newest search of each Slot of
// the Title (or of slot_id), winners first, then passed over, then
// refused; paged with limit and offset.
func (a *API) titleReleases(w http.ResponseWriter, r *http.Request) {
	t, ok := a.titleID(w, r)
	if !ok {
		return
	}
	n, off := page(r, 100)
	db := a.s.Store.Read()
	slotFilter := ""
	var slotArg []any
	for _, f := range []struct{ param, cond string }{{"slot_id", " AND s.id = ?"}, {"item_id", " AND i.id = ?"},
		{"season", " AND json_extract(i.numbers, '$.season') = ?"}, {"version_id", " AND s.version_id = ?"}, {"album_id", " AND i.part_id = ?"}} {
		sid := r.URL.Query().Get(f.param)
		if sid == "" {
			continue
		}
		id, err := strconv.ParseInt(sid, 10, 64)
		if err != nil {
			fail(w, http.StatusBadRequest, f.param+" must be a number")
			return
		}
		slotFilter += f.cond
		slotArg = append(slotArg, id)
	}
	args := append(append([]any{t.ID}, slotArg...), append([]any{source.KindSearchSlot, t.ID}, slotArg...)...)
	args = append(args, append([]any{source.KindSearchSeason, source.KindSearchAlbum, t.ID}, slotArg...)...)
	// The newest search Job per Slot of this Title: one whose Releases
	// matched the Slot, or a search of the Slot itself, so Releases that
	// matched no Title are listed too.
	rows, err := db.QueryContext(r.Context(), `SELECT max(job_id) FROM (
		SELECT s.id AS slot, r.job_id AS job_id FROM releases r JOIN json_each(r.matched_slot_ids) j
			JOIN slots s ON s.id = j.value JOIN items i ON i.id = s.item_id
			WHERE i.title_id = ? AND r.job_id IS NOT NULL AND r.found_by != 'feed'`+slotFilter+`
		UNION ALL
		SELECT s.id, jb.id FROM jobs jb JOIN slots s ON s.id = json_extract(jb.subject, '$.id') JOIN items i ON i.id = s.item_id
			WHERE jb.kind = ? AND json_extract(jb.subject, '$.type') = 'slot' AND i.title_id = ?`+slotFilter+`
			AND EXISTS (SELECT 1 FROM releases r WHERE r.job_id = jb.id)
		UNION ALL
		SELECT s.id, jb.id FROM jobs jb JOIN items i ON i.part_id = json_extract(jb.subject, '$.id')
			JOIN slots s ON s.item_id = i.id AND s.version_id = json_extract(jb.params, '$.version_id')
			WHERE jb.kind IN (?, ?) AND json_extract(jb.subject, '$.type') = 'part' AND i.title_id = ?`+slotFilter+`
			AND EXISTS (SELECT 1 FROM releases r WHERE r.job_id = jb.id)
	) GROUP BY slot`, args...)
	if err != nil {
		internal(w, err)
		return
	}
	var jobIDs []any
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			internal(w, err)
			return
		}
		jobIDs = append(jobIDs, id)
	}
	rows.Close()
	out := []ReleaseView{}
	total := 0
	var running []int64
	if len(jobIDs) > 0 {
		in := "job_id IN (" + strings.TrimSuffix(strings.Repeat("?,", len(jobIDs)), ",") + ")"
		if err := db.QueryRowContext(r.Context(), "SELECT count(*) FROM releases WHERE "+in, jobIDs...).Scan(&total); err != nil {
			internal(w, err)
			return
		}
		rels, err := store.Select[domain.Release](r.Context(), db, "releases", in+` ORDER BY CASE state WHEN 'chosen' THEN 0 WHEN 'held' THEN 1
			WHEN 'judged' THEN 2 WHEN 'passed_over' THEN 3 WHEN 'turned_down' THEN 5 ELSE 4 END,
			COALESCE(json_extract(verdict, '$.rank'), 9999), id LIMIT ? OFFSET ?`, append(jobIDs, n, off)...)
		if err != nil {
			internal(w, err)
			return
		}
		for _, x := range rels {
			out = append(out, releaseView(x))
		}
		a.named(r.Context(), out)
	}
	// Searches of this Title still running, so a screen can wait for them.
	js, err := store.Select[domain.Job](r.Context(), db, "jobs", `state IN ('queued', 'running') AND (
		kind = ? AND json_extract(subject, '$.type') = 'slot'
			AND json_extract(subject, '$.id') IN (SELECT s.id FROM slots s JOIN items i ON i.id = s.item_id WHERE i.title_id = ?)
		OR kind IN (?, ?) AND json_extract(subject, '$.type') = 'part' AND json_extract(subject, '$.id') IN (SELECT id FROM parts WHERE title_id = ?)) ORDER BY id`,
		source.KindSearchSlot, t.ID, source.KindSearchSeason, source.KindSearchAlbum, t.ID)
	if err == nil {
		for _, j := range js {
			running = append(running, j.ID)
		}
	}
	if running == nil {
		running = []int64{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"releases": out, "total": total, "limit": n, "offset": off, "running_job_ids": running})
}

func (a *API) searcher() *source.Searcher { return a.s.Search }

func searchError(w http.ResponseWriter, err error) {
	var nf *source.NeedsForceError
	switch {
	case errors.As(err, &nf):
		writeJSON(w, http.StatusConflict, map[string]any{"error": nf.Error(), "needs_force": true, "reasons": nf.Reasons})
	case errors.Is(err, source.ErrNoRelease):
		fail(w, http.StatusNotFound, err.Error())
	case errors.Is(err, source.ErrState):
		fail(w, http.StatusNotFound, err.Error())
	case errors.Is(err, source.ErrNoSlot):
		fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, source.ErrBusy):
		fail(w, http.StatusConflict, err.Error())
	default:
		internal(w, err)
	}
}

// chooseRelease sends a Release a person picked. Body (optional):
// {"force": bool, "slot_id": int}.
func (a *API) chooseRelease(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Force  bool  `json:"force"`
		SlotID int64 `json:"slot_id"`
	}
	if r.ContentLength != 0 && !decode(w, r, &in, `body is {"force": bool, "slot_id": int}, both optional`) {
		return
	}
	ch, err := a.searcher().Choose(r.Context(), id, in.SlotID, in.Force, actor(r))
	if err != nil {
		searchError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ch)
}

// turnDownRelease puts a Release on the Turned down list. Body (optional):
// {"reason": text}.
func (a *API) turnDownRelease(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Reason string `json:"reason"`
		SlotID int64  `json:"slot_id"`
	}
	if r.ContentLength != 0 && !decode(w, r, &in, `body is {"reason": text, "slot_id": int}, both optional`) {
		return
	}
	if len(in.Reason) > 500 {
		fail(w, http.StatusBadRequest, "reason is at most 500 characters")
		return
	}
	if err := a.searcher().TurnDownFor(r.Context(), id, in.SlotID, strings.TrimSpace(in.Reason), actor(r)); err != nil {
		searchError(w, err)
		return
	}
	var rel domain.Release
	if err := store.Get(r.Context(), a.s.Store.Read(), "releases", id, &rel); err != nil {
		internal(w, err)
		return
	}
	v := []ReleaseView{releaseView(rel)}
	a.named(r.Context(), v)
	writeJSON(w, http.StatusOK, v[0])
}

func (a *API) listState(state string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		n, off := page(r, 50)
		list, total, err := a.searcher().List(r.Context(), state, n, off)
		if err != nil {
			internal(w, err)
			return
		}
		out := make([]ReleaseView, 0, len(list))
		for _, x := range list {
			out = append(out, releaseView(x.Release))
		}
		a.named(r.Context(), out)
		writeJSON(w, http.StatusOK, map[string]any{"releases": out, "total": total, "limit": n, "offset": off})
	}
}

func (a *API) removeFrom(state string, one bool) http.HandlerFunc {
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
			n, err = a.searcher().RemoveHeld(r.Context(), id, actor(r))
		} else {
			n, err = a.searcher().RemoveTurnedDown(r.Context(), id, actor(r))
		}
		if err != nil {
			searchError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"removed": n})
	}
}

// bulkSearch queues one search_missing Job for the whole Missing list or
// the whole Better copy list.
func (a *API) bulkSearch(which string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, _, err := a.s.Queue.Enqueue(r.Context(), source.KindSearchMissing, nil, map[string]any{"which": which}, "search_missing:"+which, actor(r))
		if err != nil {
			internal(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"job_ids": []int64{id}})
	}
}

func (a *API) feedCheckNow(w http.ResponseWriter, r *http.Request) {
	id, _, err := a.s.Queue.Enqueue(r.Context(), source.KindFeedCheck, nil, map[string]any{}, "schedule:"+source.KindFeedCheck, actor(r))
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_ids": []int64{id}})
}

// SearchSettings are the search choices a person can change.
type SearchSettings struct {
	FeedCheckMinutes    int  `json:"feed_check_minutes"`    // 0 is off, else 10 to 120
	MissingSearchCount  int  `json:"missing_search_count"`  // per daily run; 0 is off, at most 500
	MissingSearchBetter bool `json:"missing_search_better"` // the daily run also looks for better copies
	TryNextDownloader   bool `json:"try_next_downloader"`
	// Shows (stage 3.2).
	MissingSearchShows bool   `json:"missing_search_shows"` // the daily and bulk searches cover episodes
	SeasonPackUpgrade  string `json:"season_pack_upgrade"`  // all: every episode a pack holds must gain; any: one is enough
	SeasonPackPrefer   string `json:"season_pack_prefer"`   // rank: a pack wins ties; always: a passing pack wins
	MultiSeasonPacks   bool   `json:"multi_season_packs"`   // Releases with several seasons may be taken
	// Music (stage 3.3).
	MissingSearchMusic    bool `json:"missing_search_music"`    // the daily and bulk searches cover albums
	MusicDiscographyPacks bool `json:"music_discography_packs"` // a whole-discography Release may fill an album
	AlbumTrackTolerance   int  `json:"album_track_tolerance"`   // files a Release may lack against the album's tracks; -1 off
	AlbumExtraFiles       int  `json:"album_extra_files"`       // files a bare file count is taken to hold besides tracks; 0 to 20
	ArtistSearchCap       int  `json:"artist_search_cap"`       // most album searches one Search now on an artist queues; 1 to 2000
	// Early and fake Releases (4.3.5).
	RefuseEarlyReleases    bool `json:"refuse_early_releases"`     // a Release posted before its movie, episode or album came out is refused
	EarlyReleaseGraceHours int  `json:"early_release_grace_hours"` // 0 to 720
	RefuseTinyReleases     bool `json:"refuse_tiny_releases"`      // far too small for what it holds, or an empty or old-container file at import
	ImportMinVideoMB       int  `json:"import_min_video_mb"`       // least video file placed; 0 uses the fixed floor (1 MB a minute, 20 MB unknown length)
	ImportMinTrackKB       int  `json:"import_min_track_kb"`       // least track file placed; 0 uses the fixed floor (512 KB)
	ImportSizeFloors       bool `json:"import_size_floors"`        // the fixed least sizes at import and the judged-size check; off leaves empty files and the sizes set above
	HoldUnknownReleases    bool `json:"hold_unknown_releases"`     // automatic searches hold a Release of unknown size or dates for a person; off warns only
	EpisodeDefaultMinutes  int  `json:"episode_default_minutes"`   // length taken for an episode of unknown length by the size floor; 1 to 240
}

func (a *API) readSearchSettings(ctx context.Context) (SearchSettings, error) {
	db := a.s.Store.Read()
	var st SearchSettings
	var every int
	var enabled bool
	err := db.QueryRowContext(ctx, "SELECT every_seconds, enabled FROM schedules WHERE kind = ?", source.KindFeedCheck).Scan(&every, &enabled)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return st, err
	}
	if enabled {
		st.FeedCheckMinutes = every / 60
	}
	var missingOn bool
	_ = db.QueryRowContext(ctx, "SELECT enabled FROM schedules WHERE kind = ?", source.KindSearchMissing).Scan(&missingOn)
	st.MissingSearchCount, _ = strconv.Atoi(source.Setting(ctx, db, source.SettingMissingCount))
	if !missingOn {
		st.MissingSearchCount = 0
	}
	st.MissingSearchBetter = source.Setting(ctx, db, source.SettingMissingBetter) == "true"
	st.TryNextDownloader = source.Setting(ctx, db, "try_next_downloader") != "false"
	st.MissingSearchShows = source.Setting(ctx, db, source.SettingMissingShows) != "false"
	st.SeasonPackUpgrade = "all"
	if source.Setting(ctx, db, source.SettingPackUpgrade) == "any" {
		st.SeasonPackUpgrade = "any"
	}
	st.SeasonPackPrefer = "rank"
	if source.Setting(ctx, db, source.SettingPackPrefer) == "always" {
		st.SeasonPackPrefer = "always"
	}
	st.MultiSeasonPacks = source.Setting(ctx, db, source.SettingMultiSeason) == "true"
	st.MissingSearchMusic = source.Setting(ctx, db, source.SettingMissingMusic) != "false"
	st.MusicDiscographyPacks = source.Setting(ctx, db, source.SettingDiscography) == "true"
	if n, err := strconv.Atoi(source.Setting(ctx, db, source.SettingTrackTolerance)); err == nil {
		st.AlbumTrackTolerance = n
	}
	st.AlbumExtraFiles, _ = strconv.Atoi(source.Setting(ctx, db, source.SettingExtraFiles))
	st.ArtistSearchCap = artistSearchCap(ctx, db)
	ch := source.Checks(ctx, db)
	st.RefuseEarlyReleases, st.EarlyReleaseGraceHours, st.RefuseTinyReleases = ch.Early, int(ch.Grace.Hours()), ch.Tiny
	st.ImportMinVideoMB, _ = strconv.Atoi(source.Setting(ctx, db, "import_min_video_mb"))
	st.ImportMinTrackKB, _ = strconv.Atoi(source.Setting(ctx, db, "import_min_track_kb"))
	st.ImportSizeFloors = source.Setting(ctx, db, source.SettingImportFloors) != "false"
	st.HoldUnknownReleases, st.EpisodeDefaultMinutes = ch.HoldUnknown, ch.EpisodeMinutes
	return st, nil
}

func (a *API) getSearchSettings(w http.ResponseWriter, r *http.Request) {
	st, err := a.readSearchSettings(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (a *API) putSearchSettings(w http.ResponseWriter, r *http.Request) {
	// Fields left out keep their value.
	in, err := a.readSearchSettings(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	if !decode(w, r, &in, "body is the search settings") {
		return
	}
	if in.SeasonPackUpgrade != "all" && in.SeasonPackUpgrade != "any" {
		fail(w, http.StatusBadRequest, "season_pack_upgrade is all or any")
		return
	}
	if in.SeasonPackPrefer != "rank" && in.SeasonPackPrefer != "always" {
		fail(w, http.StatusBadRequest, "season_pack_prefer is rank or always")
		return
	}
	if in.AlbumTrackTolerance < -1 || in.AlbumTrackTolerance > 50 {
		fail(w, http.StatusBadRequest, "album_track_tolerance is -1 (off) or 0 to 50")
		return
	}
	if in.AlbumExtraFiles < 0 || in.AlbumExtraFiles > 20 {
		fail(w, http.StatusBadRequest, "album_extra_files is 0 to 20")
		return
	}
	if in.ArtistSearchCap < 1 || in.ArtistSearchCap > 2000 {
		fail(w, http.StatusBadRequest, "artist_search_cap is 1 to 2000")
		return
	}
	if in.EarlyReleaseGraceHours < 0 || in.EarlyReleaseGraceHours > 720 {
		fail(w, http.StatusBadRequest, "early_release_grace_hours is 0 to 720")
		return
	}
	if in.ImportMinVideoMB < 0 || in.ImportMinVideoMB > 10000 || in.ImportMinTrackKB < 0 || in.ImportMinTrackKB > 100000 {
		fail(w, http.StatusBadRequest, "import_min_video_mb is 0 to 10000 and import_min_track_kb 0 to 100000")
		return
	}
	if in.EpisodeDefaultMinutes < 1 || in.EpisodeDefaultMinutes > 240 {
		fail(w, http.StatusBadRequest, "episode_default_minutes is 1 to 240")
		return
	}
	if in.FeedCheckMinutes != 0 && (in.FeedCheckMinutes < 10 || in.FeedCheckMinutes > 120) {
		fail(w, http.StatusBadRequest, "feed_check_minutes is 0 (off) or 10 to 120")
		return
	}
	if in.MissingSearchCount < 0 || in.MissingSearchCount > 500 {
		fail(w, http.StatusBadRequest, "missing_search_count is 0 (off) to 500")
		return
	}
	by := actor(r)
	now := store.FormatTime(time.Now())
	byText := actorJSONText(by)
	err = a.s.Store.Write(r.Context(), func(tx *sql.Tx) error {
		feedOn, feedEvery := in.FeedCheckMinutes > 0, in.FeedCheckMinutes*60
		if !feedOn {
			feedEvery = 15 * 60 // off keeps the default interval for when it is turned on again
		}
		if _, err := tx.Exec("UPDATE schedules SET every_seconds = ?, enabled = ?, updated_at = ?, updated_by = ? WHERE kind = ?",
			feedEvery, feedOn, now, byText, source.KindFeedCheck); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE schedules SET enabled = ?, updated_at = ?, updated_by = ? WHERE kind = ?",
			in.MissingSearchCount > 0, now, byText, source.KindSearchMissing); err != nil {
			return err
		}
		for k, v := range map[string]string{source.SettingMissingCount: strconv.Itoa(max(in.MissingSearchCount, 0)),
			source.SettingMissingBetter: strconv.FormatBool(in.MissingSearchBetter), "try_next_downloader": strconv.FormatBool(in.TryNextDownloader),
			source.SettingMissingShows: strconv.FormatBool(in.MissingSearchShows), source.SettingPackUpgrade: in.SeasonPackUpgrade,
			source.SettingPackPrefer: in.SeasonPackPrefer, source.SettingMultiSeason: strconv.FormatBool(in.MultiSeasonPacks),
			source.SettingMissingMusic: strconv.FormatBool(in.MissingSearchMusic), source.SettingDiscography: strconv.FormatBool(in.MusicDiscographyPacks),
			source.SettingTrackTolerance: strconv.Itoa(in.AlbumTrackTolerance), source.SettingExtraFiles: strconv.Itoa(in.AlbumExtraFiles),
			source.SettingArtistSearchCap: strconv.Itoa(in.ArtistSearchCap),
			source.SettingRefuseEarly:     strconv.FormatBool(in.RefuseEarlyReleases), source.SettingEarlyGrace: strconv.Itoa(in.EarlyReleaseGraceHours),
			source.SettingRefuseTiny: strconv.FormatBool(in.RefuseTinyReleases), "import_min_video_mb": strconv.Itoa(in.ImportMinVideoMB),
			"import_min_track_kb": strconv.Itoa(in.ImportMinTrackKB), source.SettingImportFloors: strconv.FormatBool(in.ImportSizeFloors),
			source.SettingHoldUnknown: strconv.FormatBool(in.HoldUnknownReleases), source.SettingEpisodeMinutes: strconv.Itoa(in.EpisodeDefaultMinutes)} {
			if k == source.SettingMissingCount && in.MissingSearchCount == 0 {
				continue // off keeps the last count for when it is turned on again
			}
			if _, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('what_to_pick', ?, ?, ?, ?, ?, ?)
				ON CONFLICT (grp, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
				k, v, now, byText, now, byText); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		internal(w, err)
		return
	}
	e := &domain.Event{Actor: by, Kind: "setting_changed", Summary: "Changed the search settings", Data: map[string]any{"search": in}}
	if a.s.Store.AddEvent(r.Context(), e) == nil && a.s.Live != nil {
		a.s.Live.Publish("event", e)
	}
	a.getSearchSettings(w, r)
}

func actorJSONText(a domain.Actor) string {
	b, _ := json.Marshal(a)
	return string(b)
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

var searchKinds = []any{source.KindSearchSlot, source.KindSearchMissing, source.KindFeedCheck, source.KindSearchSeason, source.KindSearchAlbum}

// searches lists the search Jobs queued or running, oldest first, and the
// newest finished ones (limit, default 10) with what each found.
func (a *API) searches(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	db := a.s.Store.Read()
	kinds := "kind IN (?, ?, ?, ?, ?)"
	running, err := store.Select[domain.Job](ctx, db, "jobs", kinds+" AND state IN ('queued', 'running') ORDER BY id LIMIT 100", searchKinds...)
	if err != nil {
		internal(w, err)
		return
	}
	recent, err := store.Select[domain.Job](ctx, db, "jobs", kinds+" AND state IN ('done', 'failed') AND ended_at IS NOT NULL ORDER BY ended_at DESC, id DESC LIMIT ?",
		append(searchKinds, limit(r, 10))...)
	if err != nil {
		internal(w, err)
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
				var tid int64
				if db.QueryRowContext(ctx, `SELECT t.id, t.name, v.label FROM slots s JOIN items i ON i.id = s.item_id JOIN titles t ON t.id = i.title_id
					JOIN versions v ON v.id = s.version_id WHERE s.id = ?`, j.Subject.ID).Scan(&tid, &v.TitleName, &v.Version) == nil {
					v.TitleID = &tid
				}
				v.Episode = a.episodeLabels(ctx, []int64{j.Subject.ID})
				v.Album = a.albumLabels(ctx, []int64{j.Subject.ID})
			}
			if j.Subject != nil && j.Subject.Type == "part" {
				tid, name, season, album := a.partLabel(ctx, j.Subject.ID)
				if tid > 0 {
					v.TitleID, v.TitleName, v.Episode, v.Album = &tid, name, season, album
				}
				if vid, ok := j.Params["version_id"].(float64); ok {
					_ = db.QueryRowContext(ctx, "SELECT label FROM versions WHERE id = ?", int64(vid)).Scan(&v.Version)
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
			case j.State == "done" && (j.Kind == source.KindSearchSlot || j.Kind == source.KindSearchSeason || j.Kind == source.KindSearchAlbum):
				v.Outcome = "none"
			}
			for _, k := range []string{"chosen_release_id", "held_release_id"} {
				if id, ok := j.Result[k].(float64); ok {
					_ = db.QueryRowContext(ctx, "SELECT name FROM releases WHERE id = ?", int64(id)).Scan(&v.Picked)
				}
			}
			out = append(out, v)
		}
		return out
	}
	writeJSON(w, http.StatusOK, map[string]any{"running": view(running), "recent": view(recent)})
}
