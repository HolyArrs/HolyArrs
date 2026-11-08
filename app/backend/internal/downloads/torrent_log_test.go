package downloads

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

func TestTorrentLogStateLines(t *testing.T) {
	t.Parallel()
	cases := []struct {
		prev, cur torrentSnap
		want      string
	}{
		{torrentSnap{detail: "downloading", progress: .5}, torrentSnap{detail: "stoppedDL", progress: .5}, "Paused"},
		{torrentSnap{detail: "stoppedDL"}, torrentSnap{detail: "downloading"}, "Resumed"},
		{torrentSnap{detail: "downloading"}, torrentSnap{detail: "stalledDL"}, "Stalled"},
		{torrentSnap{detail: "uploading", progress: 1}, torrentSnap{detail: "checkingUP", progress: 1}, "Checking"},
		{torrentSnap{detail: "uploading"}, torrentSnap{detail: "moving"}, "Moving"},
		{torrentSnap{detail: "uploading"}, torrentSnap{detail: "error"}, "Errored"},
		{torrentSnap{detail: "uploading"}, torrentSnap{detail: "missingFiles"}, "Missing files"},
		{torrentSnap{detail: "uploading", savePath: "/a"}, torrentSnap{detail: "uploading", savePath: "/b"}, "Moved to /b"},
		{torrentSnap{detail: "downloading", progress: .9}, torrentSnap{detail: "uploading", progress: 1}, "Completed"},
		{torrentSnap{detail: "uploading", progress: 1}, torrentSnap{detail: "stoppedUP", progress: 1, ratio: 2, limits: &ShareLimits{Ratio: 1, SeedingMinutes: -1}}, "Seeding rule reached"},
		{torrentSnap{detail: "uploading"}, torrentSnap{detail: "uploading", trackerProblem: "error"}, "Tracker error"},
	}
	for _, c := range cases {
		ls := stateLines(c.prev, c.cur)
		var got []string
		for _, l := range ls {
			got = append(got, l.Summary)
		}
		if !strings.Contains(strings.Join(got, "|"), c.want) {
			t.Errorf("%+v -> %+v: got %v, want %q", c.prev, c.cur, got, c.want)
		}
	}
	if ls := stateLines(torrentSnap{detail: "uploading", progress: 1}, torrentSnap{detail: "uploading", progress: 1}); len(ls) != 0 {
		t.Fatalf("no change wrote %v", ls)
	}
}

func TestTorrentLogTrackerLinesNeverKeepPasskey(t *testing.T) {
	t.Parallel()
	first := []Tracker{{URL: "https://t.example:443", Status: "working", Message: ""}}
	ls, seen := trackerLines(nil, first)
	if len(ls) != 0 {
		t.Fatalf("first sighting recorded %v", ls)
	}
	next := []Tracker{{URL: "https://t.example:443", Status: "not_working", Message: "down: http://t.example/announce?passkey=SECRET123"},
		{URL: "DHT", Fixed: true, Message: "x"}}
	ls, _ = trackerLines(seen, next)
	if len(ls) != 1 || !strings.HasPrefix(ls[0].Summary, "t.example: ") || strings.Contains(ls[0].Summary, "SECRET123") || strings.Contains(ls[0].Summary, "passkey") {
		t.Fatalf("tracker lines %+v", ls)
	}
}

func TestTorrentLogDerivedLines(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	added, done := now.Add(-15*24*time.Hour), now.Add(-15*24*time.Hour+42*time.Minute)
	d := &TorrentDetail{AddedAt: &added, DoneAt: &done, SeedingTime: int64(14 * 24 * 3600), Uploaded: 1 << 30, Ratio: .09}
	ls := derivedLines(d, now.Add(-3*24*time.Hour).Unix(), now)
	var got []string
	for _, l := range ls {
		got = append(got, l.Summary)
	}
	j := strings.Join(got, "|")
	for _, w := range []string{"Added", "Finished downloading in 42m", "Seeding since · 1.0 GB uploaded · ratio 0.09", "Last activity"} {
		if !strings.Contains(j, w) {
			t.Errorf("missing %q in %v", w, got)
		}
	}
	if strings.Contains(j, "Last seen") {
		t.Errorf("shown without a value: %v", got)
	}
	if len(derivedLines(&TorrentDetail{}, 0, now)) != 0 || derivedLines(nil, 0, now) != nil {
		t.Fatal("empty detail made lines")
	}
	m := mergeLog(ls)
	for i := 1; i < len(m); i++ {
		if m[i].At.After(m[i-1].At) {
			t.Fatal("not newest first")
		}
	}
}

// State changes the reader sees become log lines; old lines prune; tracker
// lines never keep a passkey.
func TestTorrentLogRecordsAndPrunes(t *testing.T) {
	t.Parallel()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	a := &Web{Store: s}
	ctx := context.Background()
	prev := Job{ID: outHash, Detail: "downloading", Progress: .5}
	cur := prev
	cur.Detail = "stoppedDL"
	if l := TorrentLines(prev, cur); len(l) != 1 || l[0].Summary != "Paused" {
		t.Fatalf("pause lines %+v", l)
	}
	if l := TorrentLines(cur, cur); len(l) != 0 {
		t.Fatalf("no change wrote %+v", l)
	}
	for i := 0; i < torrentLogCap+20; i++ {
		a.writeTorrentLog(ctx, 7, outHash, time.Now().UTC().Add(time.Duration(i)*time.Second), []DownloadLogLine{{Kind: "x", Summary: "line"}})
	}
	var n int
	_ = s.Read().QueryRow("SELECT count(*) FROM torrent_log WHERE downloader_id = 7 AND hash = ?", outHash).Scan(&n)
	if n != torrentLogCap {
		t.Fatalf("kept %d", n)
	}
	a.noteTrackers(ctx, 7, seedHash, []Tracker{{URL: "https://t.example", Message: "ok"}})
	a.noteTrackers(ctx, 7, seedHash, []Tracker{{URL: "https://t.example", Message: "unregistered torrent passkey=abc http://t.example/a/abc"}})
	var sum string
	_ = s.Read().QueryRow("SELECT summary FROM torrent_log WHERE kind = 'tracker'").Scan(&sum)
	if !strings.HasPrefix(sum, "t.example: ") || strings.Contains(sum, "/a/abc") {
		t.Fatalf("tracker line %q", sum)
	}
}

// A remove through HolyArrs and a Downloader delete drop a torrent's
// tracker memory and rows.
func TestTorrentLogForgetsGoneTorrents(t *testing.T) {
	t.Parallel()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	a := &Web{Store: s}
	ctx := context.Background()
	a.noteTrackers(ctx, 7, strings.ToUpper(routeOwnHash), []Tracker{{URL: "https://t.example", Message: "ok"}})
	a.forgetTorrents(ctx, 7, []string{routeOwnHash})
	if _, ok := a.tlog.trackers[clientKey{7, routeOwnHash}]; ok {
		t.Fatal("removed torrent's trackers kept in memory")
	}
	count := func(q string, args ...any) int {
		var n int
		_ = s.Read().QueryRow(q, args...).Scan(&n)
		return n
	}
	a.writeTorrentLog(ctx, 7, outHash, time.Now().UTC(), []DownloadLogLine{{Kind: "x", Summary: "a"}})
	a.writeTorrentLog(ctx, 8, outHash, time.Now().UTC(), []DownloadLogLine{{Kind: "x", Summary: "a"}})
	a.forgetTorrents(ctx, 7, []string{strings.ToUpper(outHash)})
	if n := count("SELECT count(*) FROM torrent_log WHERE downloader_id = 7"); n != 0 {
		t.Fatalf("removed torrent kept %d lines", n)
	}
	a.forgetTorrents(ctx, 8, nil)
	if n := count("SELECT count(*) FROM torrent_log WHERE downloader_id = 8"); n != 0 {
		t.Fatalf("deleted Downloader kept %d lines", n)
	}
}

// Removals match lower-case hashes in batches.
func TestForgetTorrentsInBatches(t *testing.T) {
	t.Parallel()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	a := &Web{Store: s}
	ctx := context.Background()
	var n int
	hashes := make([]string, 1200)
	for i := range hashes {
		hashes[i] = fmt.Sprintf("%040x", i)
	}
	_ = s.Write(ctx, func(tx *sql.Tx) error {
		for _, h := range hashes {
			if _, err := tx.Exec("INSERT INTO torrent_log (downloader_id, hash, at, kind, summary) VALUES (3, ?, '', 'x', 'a')", h); err != nil {
				return err
			}
		}
		return nil
	})
	up := make([]string, len(hashes))
	for i, h := range hashes {
		up[i] = strings.ToUpper(h)
	}
	a.forgetTorrents(ctx, 3, up)
	_ = s.Read().QueryRow("SELECT count(*) FROM torrent_log WHERE downloader_id = 3").Scan(&n)
	if n != 0 {
		t.Fatalf("batched forget left %d rows", n)
	}
}
