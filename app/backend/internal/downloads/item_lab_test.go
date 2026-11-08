//go:build lab

package downloads

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestLabQbitClientItem(t *testing.T) {
	base := os.Getenv("HOLYARRS_LAB_QBIT")
	if base == "" {
		base = "http://qbittorrent:8090"
	}
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	var did int64
	err = s.Write(ctx, func(tx *sql.Tx) error {
		d := &domain.Downloader{Name: "Lab qBittorrent", Adapter: "qbittorrent", Protocol: "torrent", URL: base, Category: "holyarrs", Priority: 1,
			PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok", AllowLocal: true, Writable: true}
		_, err := database.Insert(tx, "downloaders", d, domain.System)
		did = d.ID
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	m := &Manager{Store: s}
	q := NewQbittorrent(base, nil)
	id, err := q.Add(ctx, AddRequest{URL: "http://fake-indexer/dune-part-two.torrent", Category: "labtest-outside",
		SavePath: "/data/downloads/incomplete/labtest-outside", InfoHash: hash})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = q.Remove(context.Background(), id, true) }()
	find := func() (Job, bool) {
		all, err := q.ListAll(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, j := range all {
			if j.ID == id {
				return j, true
			}
		}
		return Job{}, false
	}
	wait := func(what string, ok func(Job, bool) bool) Job {
		t.Helper()
		deadline := time.Now().Add(90 * time.Second)
		for time.Now().Before(deadline) {
			if j, found := find(); ok(j, found) {
				return j
			}
			time.Sleep(500 * time.Millisecond)
		}
		t.Fatalf("never %s", what)
		return Job{}
	}
	j := wait("listed", func(_ Job, found bool) bool { return found })
	if j.Category != "labtest-outside" {
		t.Fatalf("category %q", j.Category)
	}
	own, _ := q.List(ctx, "holyarrs")
	for _, o := range own {
		if o.ID == id {
			t.Fatal("the holyarrs category list holds the outside torrent")
		}
	}
	act := func(f func(c Controller, id string) error) {
		t.Helper()
		if err := m.ControlItem(ctx, did, id, true, func(e Engine, h string) error { return f(e.(Controller), h) }); err != nil {
			t.Fatal(err)
		}
	}
	act(func(c Controller, h string) error { return c.PauseItem(ctx, h) })
	j = wait("stopped", func(j Job, _ bool) bool { return j.Detail == "stoppedDL" || j.Detail == "stoppedUP" })
	t.Logf("after pause: %s (%s)", j.State, j.Detail)
	act(func(c Controller, h string) error { return c.ResumeItem(ctx, h) })
	j = wait("started", func(j Job, _ bool) bool { return j.Detail != "stoppedDL" && j.Detail != "stoppedUP" })
	t.Logf("after resume: %s (%s)", j.State, j.Detail)
	if err := m.ControlItem(ctx, did, "all", true, func(Engine, string) error { return nil }); err != ErrNotFound {
		t.Fatalf("all: %v", err)
	}
	if err := m.ControlItem(ctx, did, id, true, func(e Engine, h string) error { return e.Remove(ctx, h, true) }); err != nil {
		t.Fatal(err)
	}
	wait("removed", func(_ Job, found bool) bool { return !found })
	var n int
	_ = s.Read().QueryRow("SELECT COUNT(*) FROM downloads").Scan(&n)
	if n != 0 {
		t.Fatalf("%d download records made", n)
	}
}
