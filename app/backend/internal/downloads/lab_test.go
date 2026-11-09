//go:build lab

package downloads

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestLabQbittorrent(t *testing.T) {
	base := os.Getenv("HOLYARRS_LAB_QBIT")
	if base == "" {
		base = "http://qbittorrent:8090"
	}
	q := NewQbittorrent(base, nil)
	ctx := context.Background()
	h, err := q.Health(ctx)
	if err != nil {
		t.Fatalf("health: %v (a sign-in error means the trusted subnet is not set)", err)
	}
	t.Logf("qBittorrent %s", h.Version)
	id, err := q.Add(ctx, AddRequest{URL: "http://fake-indexer/dune-part-two.torrent", Category: "holyarrs-labtest",
		SavePath: "/data/downloads/incomplete/holyarrs-labtest", InfoHash: hash})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = q.Remove(context.Background(), id, true) }()
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		j, err := q.Get(ctx, id)
		if err == nil && j.State == StateFinished {
			t.Logf("state %s (%s), progress %.2f, files %d", j.State, j.Detail, j.Progress, len(j.Files))
			list, err := q.List(ctx, "holyarrs-labtest")
			if err != nil || len(list) != 1 {
				t.Fatalf("list %v %v", list, err)
			}
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatal("the torrent never finished")
}
