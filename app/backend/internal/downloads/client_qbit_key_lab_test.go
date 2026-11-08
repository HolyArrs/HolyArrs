//go:build lab

package downloads

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func TestLabQbitAPIKey(t *testing.T) {
	key := os.Getenv("HOLYARRS_LAB_QBIT_KEY")
	if key == "" {
		t.Skip("HOLYARRS_LAB_QBIT_KEY not set (run through labctl qbt-apikey)")
	}
	base := os.Getenv("HOLYARRS_LAB_QBIT")
	if base == "" {
		base = "http://127.0.0.1:18708"
	}
	ctx := context.Background()
	if _, err := NewQbittorrentKey(base, "qbt_wrongwrongwrongwrongwrongwron", nil).Health(ctx); !errors.Is(err, ErrAuth) {
		t.Fatalf("a wrong key must be refused: %v", err)
	}
	q := NewQbittorrentKey(base, key, nil)
	h, err := q.Health(ctx)
	if err != nil {
		t.Fatalf("health with the key: %v", err)
	}
	t.Logf("qBittorrent %s", h.Version)
	st, err := q.Status(ctx)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	t.Logf("free space %d, session down %d", st.FreeSpace, st.SessionDown)
	if st.FreeSpace <= 0 {
		t.Errorf("free space not reported: %+v", st)
	}

	id, err := q.Add(ctx, AddRequest{URL: "http://fake-indexer/dune-part-two.torrent", Category: "holyarrs-labtest",
		SavePath: "/data/downloads/incomplete/holyarrs-labtest", InfoHash: hash})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := q.Remove(context.Background(), id, true); err != nil {
			t.Errorf("remove: %v", err)
		}
	}()
	wait := func(what string, ok func() bool) {
		t.Helper()
		deadline := time.Now().Add(90 * time.Second)
		for time.Now().Before(deadline) {
			if ok() {
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
		t.Fatalf("never %s", what)
	}
	var files []TorrentFile
	wait("listed its files", func() bool {
		files, err = q.TorrentFiles(ctx, id)
		return err == nil && len(files) > 0
	})
	t.Logf("files %+v", files)

	p, err := q.Properties(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if p.Size <= 0 || p.SavePath == "" || p.AddedAt == nil {
		t.Fatalf("properties %+v", p)
	}
	t.Logf("properties %+v", p)
	tr, err := q.Trackers(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	ps, err := q.Peers(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("trackers %+v, peers %d", tr, len(ps))

	if err := q.SetFilePriority(ctx, id, []int{files[0].Index}, 0); err != nil {
		t.Fatal(err)
	}
	wait("skipped the file", func() bool {
		fs, err := q.TorrentFiles(ctx, id)
		return err == nil && len(fs) > 0 && fs[0].Priority == 0
	})
	if err := q.SetFilePriority(ctx, id, []int{files[0].Index}, 6); err != nil {
		t.Fatal(err)
	}
	wait("set the file to high", func() bool {
		fs, err := q.TorrentFiles(ctx, id)
		return err == nil && len(fs) > 0 && fs[0].Priority == 6
	})

	state := func() string {
		j, err := q.Get(ctx, id)
		if err != nil {
			return ""
		}
		return j.Detail
	}
	if err := q.PauseItem(ctx, id); err != nil {
		t.Fatal(err)
	}
	wait("stopped", func() bool { s := state(); return s == "stoppedDL" || s == "stoppedUP" })
	if err := q.ResumeItem(ctx, id); err != nil {
		t.Fatal(err)
	}
	wait("started", func() bool { s := state(); return s != "" && s != "stoppedDL" && s != "stoppedUP" })
}
