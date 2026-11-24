package sources

import (
	"context"
	"testing"
)

// The Choose a release list for one episode leaves out Releases matched
// only to other episodes; ones matched to none stay, as before.
func TestReleasesOfJobs_OnlyTheEpisodeAsked(t *testing.T) {
	f := seed(t, "http://127.0.0.1:1/api")
	job := addSearchJob(t, f)
	own, other, none := addRelease(t, f, "own", "judged"), addRelease(t, f, "other", "passed_over"), addRelease(t, f, "none", "refused")
	exec(t, f, "UPDATE releases SET job_id = ?, matched_slot_ids = json_array(?) WHERE id = ?", job, f.slotID, own)
	exec(t, f, "UPDATE releases SET job_id = ?, matched_slot_ids = json_array(?) WHERE id = ?", job, f.slotID+1000, other)
	exec(t, f, "UPDATE releases SET job_id = ?, matched_slot_ids = '[]' WHERE id = ?", job, none)
	rels, total, err := releasesOfJobs(context.Background(), f.s.Read(), []any{job}, []int64{f.slotID}, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	got := map[int64]bool{}
	for _, r := range rels {
		got[r.ID] = true
	}
	if total != 2 || len(rels) != 2 || !got[own] || !got[none] || got[other] {
		t.Fatalf("total %d, got %v; want own %d and none %d, not other %d", total, got, own, none, other)
	}
	if _, total, _ := releasesOfJobs(context.Background(), f.s.Read(), []any{job}, nil, 100, 0); total != 3 {
		t.Fatalf("without an episode every Release is listed, got %d", total)
	}
}
