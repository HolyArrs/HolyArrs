package sources

import (
	"context"
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
