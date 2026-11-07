package subtitles

import (
	"context"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/jobs"
)

// New default languages list what every File misses without asking any
// site: the look Job reads the Files never looked at, in batches.
func TestLookJobListsMissingWithoutSites(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	s := &Service{Store: f.st}
	q := jobs.New(f.st, jobs.Options{Workers: 1, PollInterval: 5 * time.Millisecond})
	s.Register(q)
	if err := q.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(q.Stop)
	old := lookBatch
	lookBatch = 1 // one File a Job: the rest go to the next Job
	t.Cleanup(func() { lookBatch = old })

	wishAll(t, f, Want{Language: "es"})
	if err := s.QueueLook(ctx); err != nil {
		t.Fatal(err)
	}
	var total int
	for range 400 {
		_, total, _ = MissingList(ctx, f.st.Read(), MissingFilter{Language: "es"}, 50, 0)
		var open int
		_ = f.st.Read().QueryRow("SELECT count(*) FROM jobs WHERE kind = ? AND state IN ('queued', 'running')", KindLook).Scan(&open)
		if total >= 2 && open == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if total < 2 {
		t.Fatalf("missing Spanish rows %d", total)
	}
	var n int
	if err := f.st.Read().QueryRow("SELECT count(*) FROM jobs WHERE kind = ? AND state = 'done'", KindLook).Scan(&n); err != nil || n < 2 {
		t.Fatalf("look Jobs done %d %v", n, err)
	}
	if err := f.st.Read().QueryRow("SELECT count(*) FROM jobs WHERE kind = ?", KindSearch).Scan(&n); err != nil || n != 0 {
		t.Fatalf("a look queued %d searches", n)
	}
	var sites int
	_ = f.st.Read().QueryRow("SELECT count(*) FROM subtitle_sites").Scan(&sites)
	if sites != 0 {
		t.Fatal("no site should exist")
	}
}
