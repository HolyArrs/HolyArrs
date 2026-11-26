//go:build lab

package downloads

import (
	"context"
	"os"
	"testing"
	"time"
)

// A torrent stopped in qBittorrent (pausedDL on 4.x, stoppedDL on 5.x)
// stays in HolyArrs's category list, by torrents/info and by sync/maindata,
// so it is never taken for a removal. Run against both lab clients:
// HOLYARRS_LAB_QBIT=http://127.0.0.1:18708 (new) and http://127.0.0.1:18748 (old)
// go test -tags lab -run LabQbitStoppedStaysListed ./internal/downloads/
func TestLabQbitStoppedStaysListed(t *testing.T) {
	base := os.Getenv("HOLYARRS_LAB_QBIT")
	if base == "" {
		t.Skip("set HOLYARRS_LAB_QBIT")
	}
	const cat = "holyarrs-labtest"
	q := NewQbittorrent(base, nil)
	q.cats = []string{cat}
	ctx := context.Background()
	h, err := q.Health(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("qBittorrent %s", h.Version)
	id, err := q.Add(ctx, AddRequest{URL: "http://fake-indexer/dune-part-two.torrent", Category: cat,
		SavePath: "/data/downloads/incomplete/" + cat, InfoHash: hash})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = q.Remove(context.Background(), id, true) }()
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := q.Get(ctx, id); err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err := q.PauseItem(ctx, id); err != nil {
		t.Fatal(err)
	}
	var j Job
	for time.Now().Before(deadline) {
		if j, err = q.Get(ctx, id); err == nil && labStopped(j.Detail) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !labStopped(j.Detail) {
		t.Fatalf("never stopped: %+v", j)
	}
	list, err := listCategories(ctx, q, []string{cat})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := jobIndex(list)[id]
	if !ok || got.State != StatePaused && got.State != StateFinished {
		t.Fatalf("stopped torrent (%s) not followed in the category list: %v %+v", j.Detail, ok, got)
	}
	md, err := q.SyncMain(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	var st MainState
	if _, err := st.Apply(md); err != nil {
		t.Fatal(err)
	}
	if _, ok := jobIndex(st.Jobs(cat))[id]; !ok {
		t.Fatalf("stopped torrent missing from sync/maindata")
	}
	t.Logf("stopped as %s -> %s, alt id %q, listed by info and maindata", got.Detail, got.State, got.AltID)
}
