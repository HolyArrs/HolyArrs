package shows

import (
	"context"
	"database/sql"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"testing"
)

func TestInfoRetainsIssueReasons(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	ti := seedShow(t, s, labShow("tmdb", "100"), library.SeasonsAll)
	eps := episodes(t, s, ti.ID)
	err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE slots SET state='needs_a_decision', last_reason='Downloaded file not found' WHERE id IN (?,?)", eps["S01E01"].SlotID, eps["S01E02"].SlotID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	info, err := InfoOf(ctx, s.Read(), ti)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Issues) != 1 || info.Issues[0].Reason != "Downloaded file not found" || info.Failed != 2 {
		t.Fatalf("%+v", info)
	}
	// Alternate episode orders use the same reasons, without a per-episode read.
	rows, err := library.OrderEpisodeRows(ctx, s.Read(), ti.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, r := range rows {
		if r.Reason == "Downloaded file not found" {
			found++
		}
	}
	if found != 2 {
		t.Fatalf("alternate rows lost reasons: %d", found)
	}
}
func TestStatusIssuesAreBoundedAndCurrent(t *testing.T) {
	var issues []StatusIssue
	issues = addStatusIssue(issues, "in_library", "Old failure")
	if len(issues) != 0 {
		t.Fatal("stale issue")
	}
	for i := 0; i < 30; i++ {
		issues = addStatusIssue(issues, "failed", string(rune('a'+i)))
	}
	if len(issues) != 8 {
		t.Fatalf("unbounded reasons: %d", len(issues))
	}
}

func TestStatusIssueLimitKeepsFailurePriority(t *testing.T) {
	var issues []StatusIssue
	for i := 0; i < 8; i++ {
		issues = addStatusIssue(issues, "needs_a_decision", string(rune('a'+i)))
	}
	issues = addStatusIssue(issues, "failed", "Downloader unavailable")
	found := false
	for _, issue := range issues {
		found = found || issue.State == "failed"
	}
	if !found || len(issues) != 8 {
		t.Fatalf("failure priority lost: %+v", issues)
	}
}
