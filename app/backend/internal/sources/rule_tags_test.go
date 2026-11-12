package sources

import (
	"context"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Pick rules with tags ask only the Sources sharing one of them: an
// untagged Source is skipped, a Source with the tag is asked.
func TestHandle_PickRulesTagsLimitSources(t *testing.T) {
	lf := &liveFeed{}
	lf.set(duneFeed)
	f := seed(t, lf.url(t))
	exec(t, f, `UPDATE pick_rules SET body = json_set(body, '$.tags', json('["anime"]'))`)
	j := runSearch(t, f, &Searcher{Store: f.s})
	if n := feedHits(lf); n != 0 {
		t.Fatalf("the untagged Source was asked %d times", n)
	}
	var sl mediafiles.Slot
	if err := database.Get(context.Background(), f.s.Read(), "slots", f.slotID, &sl); err != nil {
		t.Fatal(err)
	}
	if j.State != "done" || sl.LastReason == nil || *sl.LastReason != "no Source takes this title's tags: Test: its tags (none) are not in the Pick rules' tags (anime)" {
		t.Fatalf("job %s, slot %s: %v", j.State, sl.State, sl.LastReason)
	}
	exec(t, f, `UPDATE sources SET tags = '["anime"]'`)
	exec(t, f, `UPDATE titles SET tags = '["anime"]'`)
	exec(t, f, `UPDATE slots SET state = 'wanted'`)
	j = runSearch(t, f, &Searcher{Store: f.s})
	if j.State != "done" || feedHits(lf) == 0 {
		t.Fatalf("job %s, hits %d", j.State, feedHits(lf))
	}
}

// Pick rules tags are read the way Source tags are stored (lowercase), so
// "Anime" on the Pick rules still asks a Source tagged "anime".
func TestHandle_PickRulesTagsIgnoreCase(t *testing.T) {
	lf := &liveFeed{}
	lf.set(duneFeed)
	f := seed(t, lf.url(t))
	exec(t, f, `UPDATE pick_rules SET body = json_set(body, '$.tags', json('[" Anime "]'))`)
	exec(t, f, `UPDATE sources SET tags = '["anime"]'`)
	exec(t, f, `UPDATE titles SET tags = '["anime"]'`)
	j := runSearch(t, f, &Searcher{Store: f.s})
	if j.State != "done" || feedHits(lf) == 0 {
		t.Fatalf("job %s, hits %d", j.State, feedHits(lf))
	}
}

// A Source that shares a tag with the Pick rules is asked for a title
// that carries no tags: the Pick rules' tags pick the Sources on their own.
func TestHandle_PickRulesTagsAskTaggedSource(t *testing.T) {
	lf := &liveFeed{}
	lf.set(duneFeed)
	f := seed(t, lf.url(t))
	exec(t, f, `UPDATE pick_rules SET body = json_set(body, '$.tags', json('["anime"]'))`)
	exec(t, f, `UPDATE sources SET tags = '["anime"]'`)
	j := runSearch(t, f, &Searcher{Store: f.s})
	if j.State != "done" || feedHits(lf) == 0 {
		t.Fatalf("job %s, hits %d", j.State, feedHits(lf))
	}
}
