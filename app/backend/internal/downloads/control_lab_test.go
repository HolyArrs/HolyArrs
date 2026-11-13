//go:build lab

package downloads

import (
	"context"
	"os"
	"testing"
	"time"
)

// Needs the lab qBittorrent and the Dune test torrent from the lab's fake
// indexer. Run: HOLYARRS_LAB_QBIT=http://127.0.0.1:18708 go test -tags lab -run LabQbitControls ./internal/downloads/
// It reads, pauses, resumes, rechecks, sets and clears a speed limit, then
// removes the torrent and its files.
func TestLabQbitControls(t *testing.T) {
	base := os.Getenv("HOLYARRS_LAB_QBIT")
	if base == "" {
		base = "http://qbittorrent:8090"
	}
	q := NewQbittorrent(base, nil)
	q.cats = []string{"holyarrs-labtest"}
	ctx := context.Background()
	before, err := q.Status(ctx)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	t.Logf("status before: %+v", before)
	id, err := q.Add(ctx, AddRequest{URL: "http://fake-indexer/dune-part-two.torrent", Category: "holyarrs-labtest",
		SavePath: "/data/downloads/incomplete/holyarrs-labtest", InfoHash: hash})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = q.Remove(context.Background(), id, true) }()
	wait := func(what string, ok func(Job) bool) Job {
		t.Helper()
		deadline := time.Now().Add(90 * time.Second)
		for time.Now().Before(deadline) {
			if j, err := q.Get(ctx, id); err == nil && ok(j) {
				return j
			}
			time.Sleep(500 * time.Millisecond)
		}
		t.Fatalf("never %s", what)
		return Job{}
	}
	wait("added", func(Job) bool { return true })

	if err := q.PauseItem(ctx, id); err != nil {
		t.Fatal(err)
	}
	j := wait("stopped", func(j Job) bool { return j.Detail == "stoppedDL" || j.Detail == "stoppedUP" })
	t.Logf("after pause: %s (%s)", j.State, j.Detail)
	if err := q.ResumeItem(ctx, id); err != nil {
		t.Fatal(err)
	}
	j = wait("started", func(j Job) bool { return j.Detail != "stoppedDL" && j.Detail != "stoppedUP" })
	t.Logf("after resume: %s (%s)", j.State, j.Detail)
	if err := q.Recheck(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := q.Reannounce(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := q.MoveTop(ctx, id); err != nil {
		t.Logf("move to top: %v", err) // refused while the lab queue is off
	}
	tr, err := q.Trackers(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	ps, err := q.Peers(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("trackers %+v, peers %d", tr, len(ps))

	if err := q.SetSpeedLimit(ctx, 3*1048576, 1048576); err != nil {
		t.Fatal(err)
	}
	st, err := q.Status(ctx)
	if err != nil || st.SpeedLimit != 3*1048576 || st.UpLimit != 1048576 {
		t.Fatalf("limit not applied: %+v %v", st, err)
	}
	if err := q.SetSpeedLimit(ctx, before.SpeedLimit, before.UpLimit); err != nil {
		t.Fatal(err)
	}
	if err := q.PauseAll(ctx); err != nil {
		t.Fatal(err)
	}
	wait("stopped by pause all", func(j Job) bool { return j.Detail == "stoppedDL" || j.Detail == "stoppedUP" })
	if err := q.ResumeAll(ctx); err != nil {
		t.Fatal(err)
	}
	wait("started by resume all", func(j Job) bool { return j.Detail != "stoppedDL" && j.Detail != "stoppedUP" })
	after, _ := q.Status(ctx)
	t.Logf("status after: %+v", after)
}

// Needs the lab SABnzbd. It has no Usenet server, so only the whole-client
// controls are checked. Run: HOLYARRS_LAB_SAB_KEY=<key from labctl> HOLYARRS_LAB_SAB=http://127.0.0.1:18707
// go test -tags lab -run LabSabControls ./internal/downloads/
func TestLabSabControls(t *testing.T) {
	key := os.Getenv("HOLYARRS_LAB_SAB_KEY")
	if key == "" {
		t.Skip("set HOLYARRS_LAB_SAB_KEY to the lab SABnzbd API key (labctl prints it) to run this test")
	}
	base := os.Getenv("HOLYARRS_LAB_SAB")
	if base == "" {
		t.Skip("set HOLYARRS_LAB_SAB to the lab SABnzbd address, for example http://127.0.0.1:18707")
	}
	s := NewSabnzbd(base, key, nil)
	ctx := context.Background()
	st, err := s.Status(ctx)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	t.Logf("status before: %+v", st)
	defer func() { _ = s.ResumeAll(context.Background()); _ = s.SetSpeedLimit(context.Background(), 0, 0) }()
	if err := s.PauseAll(ctx); err != nil {
		t.Fatal(err)
	}
	if st, _ = s.Status(ctx); !st.Paused {
		t.Fatal("pause all: client not paused")
	}
	if err := s.ResumeAll(ctx); err != nil {
		t.Fatal(err)
	}
	if st, _ = s.Status(ctx); st.Paused {
		t.Fatal("resume all: client still paused")
	}
	if err := s.SetSpeedLimit(ctx, 5<<20, 0); err != nil {
		t.Fatal(err)
	}
	st, _ = s.Status(ctx)
	if st.SpeedLimit < 4<<20 || st.SpeedLimit > 6<<20 {
		t.Fatalf("speed limit read back %d, want about %d", st.SpeedLimit, 5<<20)
	}
	if err := s.SetSpeedLimit(ctx, 0, 0); err != nil {
		t.Fatal(err)
	}
}
