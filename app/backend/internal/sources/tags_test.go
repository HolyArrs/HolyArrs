package sources

import (
	"context"
	"database/sql"
	"net/url"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func feedHits(lf *liveFeed) int { lf.mu.Lock(); defer lf.mu.Unlock(); return lf.hits }

// A Source with tags serves only titles sharing one of them, in searches
// (skipped, with the reason in the job's result) and in feeds (its releases are
// not matched to other titles).
func TestHandle_TaggedSourceSkippedForTitleWithoutTag(t *testing.T) {
	lf := &liveFeed{}
	lf.set(duneFeed)
	f := seed(t, lf.url(t))
	exec(t, f, `UPDATE sources SET tags = '["kids"]'`)
	j := runSearch(t, f, &Searcher{Store: f.s})
	if n := feedHits(lf); n != 0 {
		t.Fatalf("the tagged Source was asked %d times", n)
	}
	var sl mediafiles.Slot
	if err := database.Get(context.Background(), f.s.Read(), "slots", f.slotID, &sl); err != nil {
		t.Fatal(err)
	}
	if j.State != "done" || sl.LastReason == nil || *sl.LastReason != "no Source takes this title's tags: Test: its tags (kids) are not on this title" {
		t.Fatalf("job %s, slot %s: %v", j.State, sl.State, sl.LastReason)
	}

	exec(t, f, `UPDATE titles SET tags = '["kids"]'`)
	exec(t, f, `UPDATE slots SET state = 'wanted'`)
	var chosen int
	j = runSearch(t, f, &Searcher{Store: f.s, OnChosen: func(context.Context, int64, int64) { chosen++ }})
	if j.State != "done" || feedHits(lf) == 0 || chosen != 1 {
		t.Fatalf("job %s, hits %d, chosen %d", j.State, feedHits(lf), chosen)
	}
}

func TestFeed_TaggedReleasesMatchOnlyTitlesSharingTag(t *testing.T) {
	lf := &liveFeed{}
	lf.set(feedOf(item(web1080, time.Now().Add(-2*time.Hour), 50, "")))
	f := seed(t, lf.url(t))
	feedOn(t, f)
	exec(t, f, `UPDATE sources SET tags = '["kids"]'`)
	runKind(t, f, &Searcher{Store: f.s}, jobs.KindFeedCheck, nil, nil)
	if rs, _ := database.Select[domain.Release](context.Background(), f.s.Read(), "releases", "name = ?", web1080); len(rs) != 0 {
		t.Fatalf("a tagged Source's release was matched to an untagged title: %+v", rs)
	}
	exec(t, f, `UPDATE titles SET tags = '["kids"]'`)
	lf.set(feedOf(item(web1080, time.Now().Add(-time.Hour), 50, "")))
	runKind(t, f, &Searcher{Store: f.s}, jobs.KindFeedCheck, nil, nil)
	if rs, _ := database.Select[domain.Release](context.Background(), f.s.Read(), "releases", "name = ?", web1080); len(rs) != 1 {
		t.Fatalf("with the tag on the title: %d releases", len(rs))
	}
}

// A tagged Source's release is matched against every wanted title first, so a
// release for the untagged "Dune: Part Two" never lands on a look-alike "Dune"
// that shares the Source's tag.
func TestFeed_TaggedReleaseNeverLandsOnLookAlike(t *testing.T) {
	lf := &liveFeed{}
	lf.set(feedOf(item(web1080, time.Now().Add(-2*time.Hour), 50, "")))
	f := seed(t, lf.url(t))
	feedOn(t, f)
	exec(t, f, `UPDATE sources SET tags = '["kids"]'`)
	exec(t, f, `INSERT INTO titles (created_at, created_by, updated_at, updated_by, kind, name, sort_name, year, aliases, source_ids, metadata_source,
		keep_up_to_date, new_parts_policy, release_gate, gate_delay_days, why_added, status, state, gate_driver, music_types, artwork, tags)
		SELECT created_at, created_by, updated_at, updated_by, kind, 'Dune', 'dune', 2021, '[]', '{}', metadata_source,
		keep_up_to_date, new_parts_policy, release_gate, gate_delay_days, why_added, status, state, gate_driver, music_types, artwork, '["kids"]' FROM titles LIMIT 1`)
	exec(t, f, `INSERT INTO items (created_at, created_by, updated_at, updated_by, title_id, kind, keep_up_to_date)
		SELECT created_at, created_by, updated_at, updated_by, (SELECT max(id) FROM titles), kind, 1 FROM items LIMIT 1`)
	exec(t, f, `INSERT INTO versions (created_at, created_by, updated_at, updated_by, title_id, label, pick_rules_id, library_folder_id, folder_name, keep_up_to_date, is_main)
		SELECT created_at, created_by, updated_at, updated_by, (SELECT max(id) FROM titles), 'HD', pick_rules_id, library_folder_id, 'Dune (2021)', 1, 1 FROM versions LIMIT 1`)
	exec(t, f, `INSERT INTO slots (created_at, created_by, updated_at, updated_by, item_id, version_id, state)
		SELECT created_at, created_by, updated_at, updated_by, (SELECT max(id) FROM items), (SELECT max(id) FROM versions), 'wanted' FROM slots LIMIT 1`)
	runKind(t, f, &Searcher{Store: f.s}, jobs.KindFeedCheck, nil, nil)
	if rs, _ := database.Select[domain.Release](context.Background(), f.s.Read(), "releases", "name = ?", web1080); len(rs) != 0 {
		t.Fatalf("the release landed on a title: %+v", rs[0].MatchedSlotIDs)
	}
}

// The feed takes a tagged Source the way Search does: a title whose Pick
// rules share the Source's tag gets its releases even without the tag itself.
func TestFeed_TaggedSourceServesTitleThroughPickRulesTags(t *testing.T) {
	lf := &liveFeed{}
	lf.set(feedOf(item(web1080, time.Now().Add(-2*time.Hour), 50, "")))
	f := seed(t, lf.url(t))
	feedOn(t, f)
	exec(t, f, `UPDATE sources SET tags = '["kids"]'`)
	runKind(t, f, &Searcher{Store: f.s}, jobs.KindFeedCheck, nil, nil)
	if rs, _ := database.Select[domain.Release](context.Background(), f.s.Read(), "releases", "name = ?", web1080); len(rs) != 0 {
		t.Fatalf("a tagged Source's release went to a title with no tags anywhere: %d", len(rs))
	}
	exec(t, f, `UPDATE pick_rules SET body = json_set(body, '$.tags', json('[" Kids "]'))`)
	lf.set(feedOf(item(web1080, time.Now().Add(-time.Hour), 50, "")))
	runKind(t, f, &Searcher{Store: f.s}, jobs.KindFeedCheck, nil, nil)
	if rs, _ := database.Select[domain.Release](context.Background(), f.s.Read(), "releases", "name = ?", web1080); len(rs) != 1 {
		t.Fatalf("with the tag on the Pick rules: %d releases", len(rs))
	}
}

// Pick rules with tags take feed Releases only from Sources sharing one, as
// Search asks only those: an untagged Source's release does not fill the Slot.
func TestFeed_PickRulesTagsSkipUntaggedSource(t *testing.T) {
	lf := &liveFeed{}
	lf.set(feedOf(item(web1080, time.Now().Add(-2*time.Hour), 50, "")))
	f := seed(t, lf.url(t))
	feedOn(t, f)
	exec(t, f, `UPDATE pick_rules SET body = json_set(body, '$.tags', json('["kids"]'))`)
	runKind(t, f, &Searcher{Store: f.s}, jobs.KindFeedCheck, nil, nil)
	if rs, _ := database.Select[domain.Release](context.Background(), f.s.Read(), "releases", "name = ?", web1080); len(rs) != 0 {
		t.Fatalf("an untagged Source's release was used by Pick rules with tags: %d", len(rs))
	}
	exec(t, f, `UPDATE sources SET tags = '["kids"]'`)
	lf.set(feedOf(item(web1080, time.Now().Add(-time.Hour), 50, "")))
	runKind(t, f, &Searcher{Store: f.s}, jobs.KindFeedCheck, nil, nil)
	if rs, _ := database.Select[domain.Release](context.Background(), f.s.Read(), "releases", "name = ?", web1080); len(rs) != 1 {
		t.Fatalf("with the tag on the Source: %d releases", len(rs))
	}
}

// addTaggedSource copies the fixture's Source under a new name, address and tags.
func addTaggedSource(t *testing.T, f fixture, name, url string, tags []string, change func(*domain.Source)) {
	t.Helper()
	var src domain.Source
	if err := database.Get(context.Background(), f.s.Read(), "sources", f.srcID, &src); err != nil {
		t.Fatal(err)
	}
	src.ID, src.Name, src.Tags = 0, name, tags
	if url != "" {
		src.URL = url
	}
	if change != nil {
		change(&src)
	}
	if err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := database.Insert(tx, "sources", &src, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

// A Library folder set to ask only tagged Sources keeps the titles in it, the
// ones added before the switch too, on the Sources sharing the folder's tags.
// Turning the switch off, or moving the title to another folder, brings the
// untagged general Sources back. While a Source has the tag, the title stays
// off the general ones even when that Source cannot be used for this search;
// only when no Source has the tag does it reach them, and says so.
func TestHandle_FolderAsksOnlyTaggedSources(t *testing.T) {
	general := &liveFeed{}
	general.set(duneFeed)
	f := seed(t, general.url(t))
	anime := &liveFeed{}
	anime.set(duneFeed)
	addTaggedSource(t, f, "Anime", anime.url(t), []string{"anime"}, nil)
	run := func(q string, args ...any) domain.Job {
		t.Helper()
		exec(t, f, q, args...)
		exec(t, f, `UPDATE slots SET state = 'wanted'`)
		general.mu.Lock()
		general.hits = 0
		general.mu.Unlock()
		anime.mu.Lock()
		anime.hits = 0
		anime.mu.Unlock()
		return runSearch(t, f, &Searcher{Store: f.s})
	}
	// The note the newest search Event carries ("" for none).
	note := func() string {
		t.Helper()
		evs, _, err := f.s.Events(context.Background(), database.EventFilter{Limit: 1,
			Kinds: []string{EventSearchDone, EventSearchFailed, EventReleaseChosen, EventReleaseHeld}})
		if err != nil || len(evs) == 0 {
			t.Fatalf("no search Event: %v", err)
		}
		n, _ := evs[0].Data["sources_note"].(string)
		return n
	}
	const everySource = "No Source this search can use has this folder's tags, so Sources without the folder's tags were asked."
	// The title is already in the folder and has no tags of its own.
	run(`UPDATE library_folders SET new_title_tags = '["anime"]', only_tagged_sources = 1`)
	if feedHits(general) != 0 || feedHits(anime) == 0 {
		t.Fatalf("switch on: general asked %d, anime asked %d", feedHits(general), feedHits(anime))
	}
	if n := note(); n != "" {
		t.Fatalf("switch on and kept: the search says %q", n)
	}
	run(`UPDATE library_folders SET only_tagged_sources = 0`)
	if feedHits(general) == 0 {
		t.Fatal("switch off: the general Source was still left out")
	}
	run(`UPDATE library_folders SET only_tagged_sources = 1`)
	if feedHits(general) != 0 || feedHits(anime) == 0 {
		t.Fatalf("switch on again: general asked %d, anime asked %d", feedHits(general), feedHits(anime))
	}
	// Moved to a folder without the switch.
	var old int64
	if err := f.s.Read().QueryRow("SELECT id FROM library_folders").Scan(&old); err != nil {
		t.Fatal(err)
	}
	exec(t, f, `INSERT INTO library_folders (path, kind, label, health, same_disk_as, created_at, updated_at, created_by, updated_by)
		SELECT path || '2', kind, 'Other', health, same_disk_as, created_at, updated_at, created_by, updated_by FROM library_folders`)
	run(`UPDATE versions SET library_folder_id = (SELECT max(id) FROM library_folders)`)
	if feedHits(general) == 0 {
		t.Fatal("moved out of the folder: the general Source was still left out")
	}
	run(`UPDATE versions SET library_folder_id = ?`, old)
	if feedHits(general) != 0 || feedHits(anime) == 0 {
		t.Fatalf("moved back: general asked %d, anime asked %d", feedHits(general), feedHits(anime))
	}
	// A Source with the tag that this search cannot use still keeps the general ones out, as in the feed: nobody is asked, the Slot says why, and no failed search is written.
	failed := func() int {
		t.Helper()
		evs, _, err := f.s.Events(context.Background(), database.EventFilter{Limit: 200, Kinds: []string{EventSearchFailed}})
		if err != nil {
			t.Fatal(err)
		}
		return len(evs)
	}
	reason := func() string {
		t.Helper()
		var sl mediafiles.Slot
		if err := database.Get(context.Background(), f.s.Read(), "slots", f.slotID, &sl); err != nil || sl.LastReason == nil {
			t.Fatalf("slot reason: %v", err)
		}
		return *sl.LastReason
	}
	before := failed()
	for _, c := range []struct{ set, why string }{
		{`UPDATE sources SET use_for = '{"feed":true,"auto_search":false,"hand_search":true}' WHERE name = 'Anime'`, "Anime has search turned off"},
		{`UPDATE sources SET use_for = '{"feed":true,"auto_search":true,"hand_search":true}', format = 'rss' WHERE name = 'Anime'`, "Anime only has a feed"},
		{`UPDATE sources SET format = 'torznab', state = 'paused', pause_reason = 'rate_limited', paused_until = '2999-01-01T00:00:00Z' WHERE name = 'Anime'`, "Anime is paused"},
	} {
		run(c.set)
		want := "This folder only takes Sources tagged anime. None can be searched right now (" + c.why + "), so nothing was asked."
		if feedHits(general) != 0 || feedHits(anime) != 0 || reason() != want {
			t.Fatalf("%s: general asked %d, anime asked %d, reason %q", c.why, feedHits(general), feedHits(anime), reason())
		}
	}
	if n := failed(); n != before {
		t.Fatalf("%d failed searches were written for a folder kept apart", n-before)
	}
	run(`UPDATE sources SET state = 'enabled', pause_reason = NULL, paused_until = NULL WHERE name = 'Anime'`)
	run(`UPDATE library_folders SET new_title_tags = '["kids"]'`)
	if feedHits(general) == 0 {
		t.Fatal("a folder no Source shares a tag with left the title with nothing to ask")
	}
	if n := note(); n != everySource {
		t.Fatalf("a folder tag no Source has: the search says %q", n)
	}
	run(`UPDATE library_folders SET only_tagged_sources = 0`)
	if n := note(); n != "" {
		t.Fatalf("switch off: the search says %q", n)
	}
}

// noteOfPick is the note the newest chosen or held Event carries; ok is false when there is none.
func noteOfPick(t *testing.T, f fixture) (note string, ok bool) {
	t.Helper()
	evs, _, err := f.s.Events(context.Background(), database.EventFilter{Limit: 1, Kinds: []string{EventReleaseChosen, EventReleaseHeld}})
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) == 0 {
		return "", false
	}
	note, _ = evs[0].Data["sources_note"].(string)
	return note, true
}

const tagsPassedOver = "No Source this search can use has this folder's tags, so Sources without the folder's tags were asked."

// In a feed, the releases of an untagged Source are not matched to a title in
// a folder that asks only tagged Sources while any Source has one of its
// tags, with its feed on or off. Only when no Source has one do they reach
// the title, and the pick says so.
func TestFeed_FolderAsksOnlyTaggedSources(t *testing.T) {
	for _, c := range []struct {
		folder string
		feed   bool
		want   int
	}{{"anime", true, 0}, {"anime", false, 0}, {"anmie", true, 1}} {
		lf := &liveFeed{}
		lf.set(feedOf(item(web1080, time.Now().Add(-2*time.Hour), 50, "")))
		f := seed(t, lf.url(t))
		feedOn(t, f)
		exec(t, f, `UPDATE library_folders SET new_title_tags = ?, only_tagged_sources = 1`, `["`+c.folder+`"]`)
		quiet := &liveFeed{}
		quiet.set(feedOf())
		addTaggedSource(t, f, "Anime", quiet.url(t), []string{"anime"}, func(src *domain.Source) { src.UseFor.Feed = c.feed })
		runKind(t, f, &Searcher{Store: f.s}, jobs.KindFeedCheck, nil, nil)
		rs, _ := database.Select[domain.Release](context.Background(), f.s.Read(), "releases", "name = ?", web1080)
		if len(rs) != c.want {
			t.Fatalf("folder tag %s, anime Source feed %v: the title got %d releases, want %d", c.folder, c.feed, len(rs), c.want)
		}
		note, ok := noteOfPick(t, f)
		if c.want == 1 && (!ok || note != tagsPassedOver) {
			t.Fatalf("the feed pick does not say the folder tags were passed over: %q %v", note, ok)
		}
	}
}

// A season search that passes over its folder's tags says so on each pick.
func TestHandleSeason_PickSaysFolderTagsPassedOver(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string { return []string{"Copper.Valley.S01.1080p.WEB-DL.H.264-QUILL"} }}
	f := seedShow(t, ts.start(t), nil)
	exec(t, f.fixture, `UPDATE library_folders SET new_title_tags = '["anmie"]', only_tagged_sources = 1`)
	j := runKind(t, f.fixture, &Searcher{Store: f.s}, jobs.KindSearchSeason, &domain.Subject{Type: "part", ID: f.parts[1]}, map[string]any{"version_id": f.verID})
	if j.State != "done" || f.state(t, "S01E01") != mediafiles.SlotChosen {
		t.Fatalf("job %s %+v, slot %s", j.State, j.Result, f.state(t, "S01E01"))
	}
	if note, ok := noteOfPick(t, f.fixture); !ok || note != tagsPassedOver {
		t.Fatalf("the season pick says %q %v", note, ok)
	}
}

// A title with one copy in a folder that asks only tagged Sources and another
// copy in a plain folder keeps only the first copy to the tagged Sources.
func TestHandle_FolderTagsAreForTheCopyInThatFolder(t *testing.T) {
	general := &liveFeed{}
	general.set(duneFeed)
	f := seed(t, general.url(t))
	anime := &liveFeed{}
	anime.set(duneFeed)
	addTaggedSource(t, f, "Anime", anime.url(t), []string{"anime"}, nil)
	first, second := f.slotID, secondCopy(t, f)
	exec(t, f, `UPDATE library_folders SET new_title_tags = '["anime"]', only_tagged_sources = 1 WHERE label != 'Plain'`)
	search := func(slot int64) (int, int) {
		t.Helper()
		exec(t, f, `UPDATE slots SET state = 'wanted'`)
		for _, lf := range []*liveFeed{general, anime} {
			lf.mu.Lock()
			lf.hits = 0
			lf.mu.Unlock()
		}
		g := f
		g.slotID = slot
		runSearch(t, g, &Searcher{Store: f.s})
		return feedHits(general), feedHits(anime)
	}
	if g, a := search(first); g != 0 || a == 0 {
		t.Fatalf("the copy in the tagged folder: general asked %d, anime asked %d", g, a)
	}
	if g, a := search(second); g == 0 || a != 0 {
		t.Fatalf("the copy in the plain folder: general asked %d, anime asked %d", g, a)
	}
}

// A Release held from an untagged Source before its folder asked only tagged
// Sources is not chosen when the wait is over; its Slot looks again.
func TestGrabHeld_FolderNowAsksOnlyTaggedSources(t *testing.T) {
	lf := &liveFeed{}
	lf.set(feedOf())
	f := seed(t, lf.url(t))
	quiet := &liveFeed{}
	quiet.set(feedOf())
	addTaggedSource(t, f, "Anime", quiet.url(t), []string{"anime"}, nil)
	ctx := context.Background()
	hold := func() int64 {
		t.Helper()
		var id int64
		if err := f.s.Write(ctx, func(tx *sql.Tx) error {
			rel := &domain.Release{SourceID: f.srcID, Name: web1080, LinkRef: "x", Protocol: "torrent", Flags: []string{}, Parsed: map[string]any{},
				MatchedSlotIDs: []int64{f.slotID}, FoundBy: "feed", State: "held", Verdict: map[string]any{}}
			if _, err := database.Insert(tx, "releases", rel, domain.System); err != nil {
				return err
			}
			id = rel.ID
			_, err := tx.Exec("UPDATE slots SET state = 'held', held_release_id = ?, next_try_at = ? WHERE id = ?", id, database.FormatTime(time.Now().Add(-time.Minute)), f.slotID)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	state := func() mediafiles.SlotState {
		t.Helper()
		var sl mediafiles.Slot
		if err := database.Get(ctx, f.s.Read(), "slots", f.slotID, &sl); err != nil {
			t.Fatal(err)
		}
		return sl.State
	}
	sr := &Searcher{Store: f.s}
	hold()
	exec(t, f, `UPDATE library_folders SET new_title_tags = '["anime"]', only_tagged_sources = 1`)
	if n, err := sr.grabHeld(ctx, domain.System, nil, time.Now().UTC()); err != nil || n != 0 || state() != mediafiles.SlotWanted {
		t.Fatalf("the held Release of the untagged Source: chosen %d, slot %s, %v", n, state(), err)
	}
	// Without the switch it is chosen as before.
	exec(t, f, `UPDATE library_folders SET only_tagged_sources = 0`)
	hold()
	if n, err := sr.grabHeld(ctx, domain.System, nil, time.Now().UTC()); err != nil || n != 1 || state() != mediafiles.SlotChosen {
		t.Fatalf("switch off: chosen %d, slot %s, %v", n, state(), err)
	}
}
