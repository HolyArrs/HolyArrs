package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// searchWorld adds Dune with one wanted Slot, a Source and three Releases
// from one hand search Job: one that passed, one passed over, one refused.
func searchWorld(t *testing.T) (srvURL string, s *database.Store, q *jobs.Queue, titleID, slotID int64, rels map[string]int64) {
	t.Helper()
	srv, s, q := setup(t)
	var resp struct {
		Title TitleView `json:"title"`
	}
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/titles", `{"id":"tmdb:693134"}`, 201), &resp)
	titleID, slotID = resp.Title.ID, resp.Title.Slots[0].ID
	rels = map[string]int64{}
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		src := &domain.Source{Name: "Lab", Protocol: "torrent", Format: "torznab", URL: "http://x", Kinds: []string{}, Categories: map[string][]int{},
			State: "enabled", Extra: map[string]string{}}
		if _, err := database.Insert(tx, "sources", src, domain.System); err != nil {
			return err
		}
		job := &domain.Job{Kind: jobs.KindSearchSlot, Params: map[string]any{}, Priority: "normal", State: "done", RunAfter: time.Now(), AskedBy: domain.System, MaxAttempts: 1}
		if _, err := database.Insert(tx, "jobs", job, domain.System); err != nil {
			return err
		}
		for _, r := range []struct {
			name, state string
			verdict     map[string]any
		}{
			{"Dune.Part.Two.2024.1080p.WEB-DL.H.265-QUILL", "judged", map[string]any{"outcome": "chosen", "passed": true, "rank": 1, "points": 15,
				"step": map[string]any{"index": 8, "name": "1080p_web"}, "reasons": []any{
					map[string]any{"step": "hard", "rule": "gate_skipped", "effect": "warn", "detail": "the Release gate is skipped"},
					map[string]any{"step": "points", "rule": "trait:hevc", "effect": "add", "detail": "HEVC", "points": 10}}}},
			{"Dune.Part.Two.2024.720p.HDTV.x264-OTHER", "passed_over", map[string]any{"outcome": "passed_over", "passed": true, "rank": 2,
				"reasons": []any{map[string]any{"step": "rank", "rule": "lost", "effect": "rank", "detail": "lost on quality"}}}},
			{"Dune.Part.Two.2024.HDCAM.x264-BAD", "refused", map[string]any{"outcome": "refused", "reasons": []any{
				map[string]any{"step": "step", "rule": "not_allowed", "effect": "fail", "detail": "cam is not allowed in these Pick rules"}}}},
		} {
			rel := &domain.Release{SourceID: src.ID, Name: r.name, LinkRef: "http://x/" + r.name, Protocol: "torrent", Flags: []string{}, Parsed: map[string]any{},
				MatchedSlotIDs: []int64{slotID}, Verdict: r.verdict, FoundBy: "hand_search", JobID: &job.ID, State: r.state}
			id, err := database.Insert(tx, "releases", rel, domain.System)
			if err != nil {
				return err
			}
			rels[r.state] = id
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return srv.URL, s, q, titleID, slotID, rels
}

// route: POST /api/v1/releases/{id}/choose
// route: POST /api/v1/releases/{id}/turn-down
func TestReleasesListChooseAndForce(t *testing.T) {
	t.Parallel()
	u, s, q, titleID, slotID, rels := searchWorld(t)
	grabs := make(chan int64, 4)
	q.Handle(downloads.KindGrab, func(_ context.Context, r *jobs.Run) error { grabs <- r.Job.Subject.ID; return nil })
	if err := q.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer q.Stop()

	var page struct {
		Releases []sources.ReleaseView `json:"releases"`
		Total    int                   `json:"total"`
		Running  []int64               `json:"running_job_ids"`
	}
	_ = json.Unmarshal(do(t, "GET", u+"/api/v1/titles/"+itoa(titleID)+"/releases?limit=2", "", 200), &page)
	if page.Total != 3 || len(page.Releases) != 2 || page.Releases[0].ID != rels["judged"] || page.Releases[1].ID != rels["passed_over"] {
		t.Fatalf("page %+v", page)
	}
	best := page.Releases[0]
	if best.Rank != 1 || best.Step != "1080p_web" || !best.GateSkip || best.SourceName != "Lab" || best.TitleName != "Dune: Part Two" ||
		len(best.Reasons) != 2 || best.Reasons[1].Text != "HEVC (+10 points)" || best.NeedsForce {
		t.Fatalf("best %+v", best)
	}
	_ = json.Unmarshal(do(t, "GET", u+"/api/v1/titles/"+itoa(titleID)+"/releases?limit=2&offset=2", "", 200), &page)
	if len(page.Releases) != 1 || !page.Releases[0].NeedsForce || page.Releases[0].Reasons[0].Effect != "fail" {
		t.Fatalf("page 2 %+v", page)
	}
	do(t, "GET", u+"/api/v1/titles/"+itoa(titleID)+"/releases?slot_id=x", "", 400)

	// A refused Release needs force, with its reasons.
	b := do(t, "POST", u+"/api/v1/releases/"+itoa(rels["refused"])+"/choose", "", 409)
	var nf struct {
		NeedsForce bool     `json:"needs_force"`
		Reasons    []string `json:"reasons"`
	}
	_ = json.Unmarshal(b, &nf)
	if !nf.NeedsForce || len(nf.Reasons) != 1 || !strings.Contains(nf.Reasons[0], "cam") {
		t.Fatalf("needs force %s", b)
	}
	do(t, "POST", u+"/api/v1/releases/999/choose", "", 404)
	do(t, "POST", u+"/api/v1/releases/"+itoa(rels["refused"])+"/choose", "{bad", 400)

	// The passed one goes straight through and a grab is queued.
	var ch sources.Choice
	_ = json.Unmarshal(do(t, "POST", u+"/api/v1/releases/"+itoa(rels["judged"])+"/choose", "", 200), &ch)
	if ch.SlotID != slotID || ch.Forced {
		t.Fatalf("choice %+v", ch)
	}
	select {
	case id := <-grabs:
		if id != slotID {
			t.Fatalf("grab for slot %d", id)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no grab queued")
	}
	// The Slot is busy now.
	do(t, "POST", u+"/api/v1/releases/"+itoa(rels["passed_over"])+"/choose", `{"force":true}`, 409)

	// Turning one down puts it on the list with the reason.
	var tv sources.ReleaseView
	_ = json.Unmarshal(do(t, "POST", u+"/api/v1/releases/"+itoa(rels["passed_over"])+"/turn-down", `{"reason":"wrong language"}`, 200), &tv)
	if tv.State != "turned_down" || !tv.NeedsForce || !strings.Contains(tv.Reasons[len(tv.Reasons)-1].Text, "wrong language") {
		t.Fatalf("turned down %+v", tv)
	}
	do(t, "POST", u+"/api/v1/releases/999/turn-down", "", 404)
	var n int
	_ = s.Read().QueryRow("SELECT count(*) FROM releases WHERE state = 'turned_down'").Scan(&n)
	if n != 1 {
		t.Fatalf("turned down %d", n)
	}
}

// route: GET /api/v1/held
// route: DELETE /api/v1/held/{id}
// route: DELETE /api/v1/held
// route: GET /api/v1/turned-down
// route: DELETE /api/v1/turned-down/{id}
// route: DELETE /api/v1/turned-down
func TestHeldAndTurnedDownLists(t *testing.T) {
	t.Parallel()
	u, s, _, _, slotID, rels := searchWorld(t)
	ctx := context.Background()
	until := database.FormatTime(time.Now().Add(time.Hour))
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE releases SET state = 'held', hold_until = ? WHERE id IN (?, ?)", until, rels["judged"], rels["passed_over"]); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE releases SET state = 'turned_down' WHERE id = ?", rels["refused"]); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE slots SET state = 'held', held_release_id = ? WHERE id = ?", rels["judged"], slotID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var page struct {
		Releases []sources.ReleaseView `json:"releases"`
		Total    int                   `json:"total"`
		Limit    int                   `json:"limit"`
		Offset   int                   `json:"offset"`
	}
	_ = json.Unmarshal(do(t, "GET", u+"/api/v1/held?limit=1", "", 200), &page)
	if page.Total != 2 || len(page.Releases) != 1 || page.Limit != 1 || page.Releases[0].HoldUntil == nil {
		t.Fatalf("held %+v", page)
	}
	_ = json.Unmarshal(do(t, "GET", u+"/api/v1/held?limit=1&offset=1", "", 200), &page)
	if len(page.Releases) != 1 || page.Offset != 1 {
		t.Fatalf("held page 2 %+v", page)
	}
	do(t, "DELETE", u+"/api/v1/held/"+itoa(rels["judged"]), "", 200)
	do(t, "DELETE", u+"/api/v1/held/"+itoa(rels["judged"]), "", 404)
	var st string
	_ = s.Read().QueryRow("SELECT state FROM slots WHERE id = ?", slotID).Scan(&st)
	if st != "wanted" {
		t.Fatalf("slot %s after removing its Held Release", st)
	}
	var rm struct {
		Removed int `json:"removed"`
	}
	_ = json.Unmarshal(do(t, "DELETE", u+"/api/v1/held", "", 200), &rm)
	if rm.Removed != 1 {
		t.Fatalf("cleared %d", rm.Removed)
	}

	_ = json.Unmarshal(do(t, "GET", u+"/api/v1/turned-down", "", 200), &page)
	if page.Total != 1 || page.Releases[0].ID != rels["refused"] {
		t.Fatalf("turned down %+v", page)
	}
	do(t, "DELETE", u+"/api/v1/turned-down/999", "", 404)
	do(t, "DELETE", u+"/api/v1/turned-down/"+itoa(rels["refused"]), "", 200)
	_ = json.Unmarshal(do(t, "DELETE", u+"/api/v1/turned-down", "", 200), &rm)
	if rm.Removed != 0 {
		t.Fatalf("cleared %d", rm.Removed)
	}
}

// route: POST /api/v1/search/missing
// route: POST /api/v1/search/better
// route: POST /api/v1/search/feed-check
func TestBulkSearchesQueueJobs(t *testing.T) {
	t.Parallel()
	u, s, _, titleID, slotID, _ := searchWorld(t)
	// A search that only shows, for one Slot.
	var shown struct {
		JobIDs []int64 `json:"job_ids"`
	}
	_ = json.Unmarshal(do(t, "POST", u+"/api/v1/titles/"+itoa(titleID)+"/search", `{"choose":false,"slot_id":`+itoa(slotID)+`}`, 202), &shown)
	var sj domain.Job
	if len(shown.JobIDs) != 1 || database.Get(context.Background(), s.Read(), "jobs", shown.JobIDs[0], &sj) != nil || sj.Params["choose"] != false || sj.Params["by_hand"] != true {
		t.Fatalf("show search %+v %v", shown, sj.Params)
	}
	do(t, "POST", u+"/api/v1/titles/"+itoa(titleID)+"/search", `{"slot_id":999}`, 404)
	for path, kind := range map[string]string{"missing": jobs.KindSearchMissing, "better": jobs.KindSearchMissing, "feed-check": jobs.KindFeedCheck} {
		var out struct {
			JobIDs []int64 `json:"job_ids"`
		}
		_ = json.Unmarshal(do(t, "POST", u+"/api/v1/search/"+path, "", 202), &out)
		var j domain.Job
		if len(out.JobIDs) != 1 || database.Get(context.Background(), s.Read(), "jobs", out.JobIDs[0], &j) != nil || j.Kind != kind {
			t.Fatalf("%s: %+v %s", path, out, j.Kind)
		}
		if path != "feed-check" && j.Params["which"] != path {
			t.Fatalf("%s params %v", path, j.Params)
		}
	}
	// One kind gets its own Job, next to the whole-list one.
	var one struct {
		JobIDs []int64 `json:"job_ids"`
	}
	_ = json.Unmarshal(do(t, "POST", u+"/api/v1/search/missing", `{"kind":"movie"}`, 202), &one)
	var j domain.Job
	if len(one.JobIDs) != 1 || database.Get(context.Background(), s.Read(), "jobs", one.JobIDs[0], &j) != nil || j.Params["kind"] != "movie" || j.DedupeKey == nil || *j.DedupeKey != "search_missing:missing:movie" {
		t.Fatalf("kind: %+v %v", one, j.Params)
	}
	do(t, "POST", u+"/api/v1/search/missing", `{"kind":"book"}`, 400)
}

// route: GET /api/v1/settings/search
// route: PUT /api/v1/settings/search
func TestSearchSettings(t *testing.T) {
	t.Parallel()
	srv, s, _ := setup(t)
	var st sources.SearchSettings
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/settings/search", "", 200), &st)
	if st != (sources.SearchSettings{FeedCheckMinutes: 15, MissingSearchCount: 20, MissingSearchBetter: false, TryNextDownloader: true,
		MissingSearchShows: true, SeasonPackUpgrade: "all", SeasonPackPrefer: "rank", MissingSearchMusic: true, ArtistSearchCap: 200,
		RefuseEarlyReleases: true, EarlyReleaseGraceHours: 24, RefuseTinyReleases: true, ImportSizeFloors: true, EpisodeDefaultMinutes: 20}) {
		t.Fatalf("defaults %+v", st)
	}
	do(t, "PUT", srv.URL+"/api/v1/settings/search", `{"feed_check_minutes":5,"missing_search_count":20,"missing_search_better":false,"try_next_downloader":true}`, 400)
	do(t, "PUT", srv.URL+"/api/v1/settings/search", `{"feed_check_minutes":30,"missing_search_count":501,"missing_search_better":false,"try_next_downloader":true}`, 400)
	_ = json.Unmarshal(do(t, "PUT", srv.URL+"/api/v1/settings/search",
		`{"feed_check_minutes":30,"missing_search_count":0,"missing_search_better":true,"try_next_downloader":false}`, 200), &st)
	if st != (sources.SearchSettings{FeedCheckMinutes: 30, MissingSearchCount: 0, MissingSearchBetter: true, TryNextDownloader: false,
		MissingSearchShows: true, SeasonPackUpgrade: "all", SeasonPackPrefer: "rank", MissingSearchMusic: true, ArtistSearchCap: 200,
		RefuseEarlyReleases: true, EarlyReleaseGraceHours: 24, RefuseTinyReleases: true, ImportSizeFloors: true, EpisodeDefaultMinutes: 20}) {
		t.Fatalf("saved %+v", st)
	}
	var every int
	var on bool
	_ = s.Read().QueryRow("SELECT every_seconds, enabled FROM schedules WHERE kind = 'feed_check'").Scan(&every, &on)
	if every != 1800 || !on {
		t.Fatalf("feed schedule %d %v", every, on)
	}
	_ = s.Read().QueryRow("SELECT enabled FROM schedules WHERE kind = 'search_missing'").Scan(&on)
	if on {
		t.Fatal("missing search still on")
	}
	_ = json.Unmarshal(do(t, "PUT", srv.URL+"/api/v1/settings/search",
		`{"feed_check_minutes":0,"missing_search_count":20,"missing_search_better":false,"try_next_downloader":true}`, 200), &st)
	if st.FeedCheckMinutes != 0 || st.MissingSearchCount != 20 {
		t.Fatalf("off/on %+v", st)
	}
	// I4: 10 to 120 minutes are kept as given.
	_ = json.Unmarshal(do(t, "PUT", srv.URL+"/api/v1/settings/search",
		`{"feed_check_minutes":10,"missing_search_count":20,"missing_search_better":false,"try_next_downloader":true}`, 200), &st)
	_ = s.Read().QueryRow("SELECT every_seconds FROM schedules WHERE kind = 'feed_check'").Scan(&every)
	if st.FeedCheckMinutes != 10 || every != 600 {
		t.Fatalf("10 minutes stored as %d (%d s)", st.FeedCheckMinutes, every)
	}
	// Show choices (3.2.2); fields left out keep their value.
	do(t, "PUT", srv.URL+"/api/v1/settings/search", `{"season_pack_upgrade":"some"}`, 400)
	do(t, "PUT", srv.URL+"/api/v1/settings/search", `{"season_pack_prefer":"never"}`, 400)
	_ = json.Unmarshal(do(t, "PUT", srv.URL+"/api/v1/settings/search",
		`{"missing_search_shows":false,"season_pack_upgrade":"any","season_pack_prefer":"always","multi_season_packs":true}`, 200), &st)
	if st.MissingSearchShows || st.SeasonPackUpgrade != "any" || st.SeasonPackPrefer != "always" || !st.MultiSeasonPacks || st.FeedCheckMinutes != 10 || st.MissingSearchCount != 20 {
		t.Fatalf("show choices %+v", st)
	}
}

// route: GET /api/v1/searches
func TestSearchesRunningAndFound(t *testing.T) {
	t.Parallel()
	u, s, _, titleID, slotID, rels := searchWorld(t)
	note := "searching Lab"
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		run := &domain.Job{Kind: jobs.KindSearchSlot, Subject: &domain.Subject{Type: "slot", ID: slotID}, Params: map[string]any{"by_hand": true},
			Priority: "normal", State: "running", Progress: 0.5, ProgressNote: &note, RunAfter: time.Now(), AskedBy: domain.System, MaxAttempts: 1}
		if _, err := database.Insert(tx, "jobs", run, domain.System); err != nil {
			return err
		}
		ended := time.Now()
		done := &domain.Job{Kind: jobs.KindSearchSlot, Subject: &domain.Subject{Type: "slot", ID: slotID}, Params: map[string]any{}, Priority: "normal",
			State: "done", RunAfter: time.Now(), EndedAt: &ended, AskedBy: domain.System, MaxAttempts: 1,
			Result: map[string]any{"releases": 3, "refused": 1, "chosen_release_id": rels["judged"]}}
		if _, err := database.Insert(tx, "jobs", done, domain.System); err != nil {
			return err
		}
		feed := &domain.Job{Kind: jobs.KindFeedCheck, Params: map[string]any{}, Priority: "normal", State: "queued", RunAfter: time.Now(), AskedBy: domain.System, MaxAttempts: 1}
		_, err := database.Insert(tx, "jobs", feed, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var out struct {
		Running []sources.SearchView `json:"running"`
		Recent  []sources.SearchView `json:"recent"`
	}
	_ = json.Unmarshal(do(t, "GET", u+"/api/v1/searches", "", 200), &out)
	if len(out.Running) != 2 {
		t.Fatalf("running %+v", out.Running)
	}
	r := out.Running[0]
	if r.Kind != jobs.KindSearchSlot || r.TitleID == nil || *r.TitleID != titleID || r.TitleName == "" || r.Note != note || !r.ByHand || r.Progress != 0.5 {
		t.Fatalf("running search %+v", r)
	}
	if out.Running[1].Kind != jobs.KindFeedCheck || out.Running[1].TitleID != nil {
		t.Fatalf("feed check %+v", out.Running[1])
	}
	if len(out.Recent) != 1 || out.Recent[0].Found != 3 || out.Recent[0].Refused != 1 || out.Recent[0].TitleName == "" || out.Recent[0].EndedAt == nil ||
		out.Recent[0].Picked != "Dune.Part.Two.2024.1080p.WEB-DL.H.265-QUILL" || out.Recent[0].Outcome != "chosen" {
		t.Fatalf("recent %+v", out.Recent)
	}
	_ = json.Unmarshal(do(t, "GET", u+"/api/v1/searches?limit=0", "", 200), &out)
}

// A Release from the newest search of a Slot that matched no Title is still
// listed, so a person sees why it was turned away.
func TestTitleReleasesListsUnmatched(t *testing.T) {
	t.Parallel()
	u, s, _, titleID, slotID, _ := searchWorld(t)
	var unmatched int64
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		job := &domain.Job{Kind: jobs.KindSearchSlot, Subject: &domain.Subject{Type: "slot", ID: slotID}, Params: map[string]any{}, Priority: "normal",
			State: "done", RunAfter: time.Now(), AskedBy: domain.System, MaxAttempts: 1}
		if _, err := database.Insert(tx, "jobs", job, domain.System); err != nil {
			return err
		}
		rel := &domain.Release{SourceID: 1, Name: "Dune.Part.Two.2031.1080p.WEB-DL", LinkRef: "http://x/u", Protocol: "torrent", Flags: []string{}, Parsed: map[string]any{},
			MatchedSlotIDs: []int64{}, FoundBy: "hand_search", JobID: &job.ID, State: "refused", Verdict: map[string]any{"outcome": "refused", "reasons": []any{
				map[string]any{"step": "match", "rule": "none", "effect": "fail", "detail": "no Title in the library is called that"}}}}
		var err error
		unmatched, err = database.Insert(tx, "releases", rel, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var page struct {
		Releases []sources.ReleaseView `json:"releases"`
		Total    int                   `json:"total"`
	}
	_ = json.Unmarshal(do(t, "GET", u+"/api/v1/titles/"+itoa(titleID)+"/releases", "", 200), &page)
	if page.Total != 1 || len(page.Releases) != 1 || page.Releases[0].ID != unmatched || !page.Releases[0].NeedsForce {
		t.Fatalf("newest search of the Slot %+v", page)
	}
	_ = json.Unmarshal(do(t, "GET", u+"/api/v1/titles/"+itoa(titleID)+"/releases?slot_id="+itoa(slotID), "", 200), &page)
	if page.Total != 1 {
		t.Fatalf("slot filter %+v", page)
	}
}

// M3: a hand search is its own Job even while a bulk search for the same
// Slot is waiting; M4: choosing for a Slot that needs a decision is 409.
func TestHandSearchNotDedupedByBulk(t *testing.T) {
	t.Parallel()
	u, s, _, titleID, slotID, rels := searchWorld(t)
	var bulkID int64
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		key := "search_slot:" + itoa(slotID)
		j := &domain.Job{Kind: jobs.KindSearchSlot, Subject: &domain.Subject{Type: "slot", ID: slotID}, Params: map[string]any{"bulk": true},
			Priority: "normal", State: "queued", RunAfter: time.Now().Add(time.Hour), AskedBy: domain.System, MaxAttempts: 1, DedupeKey: &key}
		id, err := database.Insert(tx, "jobs", j, domain.System)
		bulkID = id
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var out struct {
		JobIDs []int64 `json:"job_ids"`
	}
	_ = json.Unmarshal(do(t, "POST", u+"/api/v1/titles/"+itoa(titleID)+"/search", `{"slot_id":`+itoa(slotID)+`}`, 202), &out)
	if len(out.JobIDs) != 1 || out.JobIDs[0] == bulkID {
		t.Fatalf("hand search folded into the bulk one: %v", out.JobIDs)
	}
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE slots SET state = 'importing' WHERE id = ?", slotID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, id := range rels {
		do(t, "POST", u+"/api/v1/releases/"+itoa(id)+"/choose", `{"force":true}`, 409)
		break
	}
}
