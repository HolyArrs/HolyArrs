package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/downloader"
	"github.com/CoderLuii/holyarrs/internal/store"
)

func TestTorrentLogStateLines(t *testing.T) {
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
		{torrentSnap{detail: "uploading", progress: 1}, torrentSnap{detail: "stoppedUP", progress: 1, ratio: 2, limits: &downloader.ShareLimits{Ratio: 1, SeedingMinutes: -1}}, "Seeding rule reached"},
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
	first := []downloader.Tracker{{URL: "https://t.example:443", Status: "working", Message: ""}}
	ls, seen := trackerLines(nil, first)
	if len(ls) != 0 {
		t.Fatalf("first sighting recorded %v", ls)
	}
	next := []downloader.Tracker{{URL: "https://t.example:443", Status: "not_working", Message: "down: http://t.example/announce?passkey=SECRET123"},
		{URL: "DHT", Fixed: true, Message: "x"}}
	ls, _ = trackerLines(seen, next)
	if len(ls) != 1 || !strings.HasPrefix(ls[0].Summary, "t.example: ") || strings.Contains(ls[0].Summary, "SECRET123") || strings.Contains(ls[0].Summary, "passkey") {
		t.Fatalf("tracker lines %+v", ls)
	}
}

func TestTorrentLogDerivedLines(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	added, done := now.Add(-15*24*time.Hour), now.Add(-15*24*time.Hour+42*time.Minute)
	d := &downloader.TorrentDetail{AddedAt: &added, DoneAt: &done, SeedingTime: int64(14 * 24 * 3600), Uploaded: 1 << 30, Ratio: .09}
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
	if len(derivedLines(&downloader.TorrentDetail{}, 0, now)) != 0 || derivedLines(nil, 0, now) != nil {
		t.Fatal("empty detail made lines")
	}
	m := mergeLog(ls)
	for i := 1; i < len(m); i++ {
		if m[i].At.After(m[i-1].At) {
			t.Fatal("not newest first")
		}
	}
}

// Outside torrents and HolyArrs torrents are recorded the same way, by
// Downloader and hash; the first sighting only remembers; old lines prune.
func TestTorrentLogRecordsAndPrunes(t *testing.T) {
	s, err := store.Open(t.TempDir(), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	a := &API{s: Server{Store: s}}
	ctx := context.Background()
	jobs := []downloader.Job{{ID: outHash, Detail: "downloading", Progress: .5}, {ID: ownHash, Detail: "uploading", Progress: 1}}
	a.noteJobs(ctx, 7, jobs)
	if n := len(a.recordedLines(ctx, 7, outHash)); n != 0 {
		t.Fatalf("first sighting wrote %d", n)
	}
	jobs[0].Detail, jobs[1].Detail = "stoppedDL", "stoppedUP"
	a.noteJobs(ctx, 7, jobs)
	a.noteJobs(ctx, 7, jobs) // no change: nothing more
	if l := a.recordedLines(ctx, 7, outHash); len(l) != 1 || l[0].Summary != "Paused" {
		t.Fatalf("outside %+v", l)
	}
	if l := a.recordedLines(ctx, 7, strings.ToUpper(ownHash)); len(l) != 1 || l[0].Summary != "Paused" {
		t.Fatalf("own %+v", l)
	}
	for i := 0; i < torrentLogCap+20; i++ {
		a.writeTorrentLog(ctx, 7, outHash, time.Now().UTC().Add(time.Duration(i)*time.Second), []DownloadLogLine{{Kind: "x", Summary: "line"}})
	}
	var n int
	_ = s.Read().QueryRow("SELECT count(*) FROM torrent_log WHERE downloader_id = 7 AND hash = ?", outHash).Scan(&n)
	if n != torrentLogCap {
		t.Fatalf("kept %d", n)
	}
	a.noteTrackers(ctx, 7, seedHash, []downloader.Tracker{{URL: "https://t.example", Message: "ok"}})
	a.noteTrackers(ctx, 7, seedHash, []downloader.Tracker{{URL: "https://t.example", Message: "unregistered torrent passkey=abc http://t.example/a/abc"}})
	var sum string
	_ = s.Read().QueryRow("SELECT summary FROM torrent_log WHERE kind = 'tracker'").Scan(&sum)
	if !strings.HasPrefix(sum, "t.example: ") || strings.Contains(sum, "/a/abc") {
		t.Fatalf("tracker line %q", sum)
	}
}

// Torrents gone from the list leave memory; a remove through HolyArrs and a
// Downloader delete drop their rows; the daily sweep drops long-unseen ones.
func TestTorrentLogForgetsGoneTorrents(t *testing.T) {
	s, err := store.Open(t.TempDir(), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	a := &API{s: Server{Store: s}}
	ctx := context.Background()
	a.noteJobs(ctx, 7, []downloader.Job{{ID: strings.ToUpper(outHash)}, {ID: ownHash}})
	a.noteTrackers(ctx, 7, ownHash, []downloader.Tracker{{URL: "https://t.example", Message: "ok"}})
	if _, ok := a.tlog.seen[clientKey{7, outHash}]; !ok {
		t.Fatal("hash not lower-cased")
	}
	a.noteJobs(ctx, 7, []downloader.Job{{ID: outHash}})
	if _, ok := a.tlog.seen[clientKey{7, ownHash}]; ok {
		t.Fatal("gone torrent kept in memory")
	}
	if _, ok := a.tlog.trackers[clientKey{7, ownHash}]; ok {
		t.Fatal("gone torrent's trackers kept in memory")
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
	old := time.Now().UTC().Add(-40 * 24 * time.Hour)
	a.writeTorrentLog(ctx, 9, seedHash, old, []DownloadLogLine{{Kind: "x", Summary: "a"}})
	a.writeTorrentLog(ctx, 9, ownHash, old, []DownloadLogLine{{Kind: "x", Summary: "a"}})
	_ = s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO torrent_seen (downloader_id, hash, seen_at) VALUES (9, ?, ?)", ownHash, time.Now().UTC().Format(store.TimeLayout))
		return err
	})
	a.sweepTorrentRows(ctx, time.Now().UTC())
	if n := count("SELECT count(*) FROM torrent_log WHERE downloader_id = 9 AND hash = ?", seedHash); n != 0 {
		t.Fatal("long-unseen torrent kept")
	}
	if n := count("SELECT count(*) FROM torrent_log WHERE downloader_id = 9 AND hash = ?", ownHash); n != 1 {
		t.Fatal("recently seen torrent swept")
	}
}

// The daily sweep runs only from clientRows after every list was read, so
// noting one Downloader never sweeps rows of another; removals match
// lower-case hashes in batches.
func TestTorrentLogSweepWaitsForEveryDownloader(t *testing.T) {
	s, err := store.Open(t.TempDir(), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	a := &API{s: Server{Store: s}}
	ctx := context.Background()
	old := time.Now().UTC().Add(-40 * 24 * time.Hour)
	a.writeTorrentLog(ctx, 2, seedHash, time.Now().UTC(), []DownloadLogLine{{Kind: "x", Summary: "a"}})
	_ = s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO torrent_seen (downloader_id, hash, seen_at) VALUES (2, ?, ?)", seedHash, old.Format(store.TimeLayout))
		return err
	})
	a.noteJobs(ctx, 1, []downloader.Job{{ID: ownHash}})
	var n int
	_ = s.Read().QueryRow("SELECT count(*) FROM torrent_log WHERE downloader_id = 2").Scan(&n)
	if n != 1 {
		t.Fatal("noting Downloader 1 swept Downloader 2's rows")
	}
	if !a.tlog.lastSweep.IsZero() {
		t.Fatal("noteJobs started the sweep")
	}
	a.maybeSweep(ctx, nil)
	_ = s.Read().QueryRow("SELECT count(*) FROM torrent_log WHERE downloader_id = 2").Scan(&n)
	if n != 0 {
		t.Fatal("sweep kept a long-unseen torrent")
	}
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

// sweepEngine lists a fixed set of jobs, or fails every list.
type sweepEngine struct {
	jobs []downloader.Job
	fail bool
}

func (e sweepEngine) Health(context.Context) (downloader.Health, error) {
	return downloader.Health{}, nil
}
func (e sweepEngine) Add(context.Context, downloader.AddRequest) (string, error) { return "", nil }
func (e sweepEngine) List(context.Context, string) ([]downloader.Job, error)    { return nil, nil }
func (e sweepEngine) Get(context.Context, string) (downloader.Job, error) {
	return downloader.Job{}, downloader.ErrNotFound
}
func (e sweepEngine) Remove(context.Context, string, bool) error { return nil }
func (e sweepEngine) ListAll(context.Context) ([]downloader.Job, error) {
	if e.fail {
		return nil, errors.New("offline")
	}
	return e.jobs, nil
}

// One offline Downloader does not stop the daily sweep: only its rows are
// kept, the healthy Downloader's long-unseen rows go, and rows keyed by a
// capitalised hash are dropped.
func TestSweepSkipsOnlyFailedDownloader(t *testing.T) {
	s, err := store.Open(t.TempDir(), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	down := addQB(t, s, "http://127.0.0.1:9")
	_ = s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE downloaders SET name = 'Offline' WHERE id = ?", down)
		return err
	})
	up := addQB(t, s, "http://127.0.0.1:10")
	m := &downloader.Manager{Store: s, Engine: func(d domain.Downloader) (downloader.Engine, error) {
		return sweepEngine{fail: d.ID == down, jobs: []downloader.Job{{ID: ownHash, Name: "x"}}}, nil
	}}
	a := &API{s: Server{Store: s, Downloads: m}}
	ctx := context.Background()
	old := time.Now().UTC().Add(-40 * 24 * time.Hour)
	for _, id := range []int64{down, up} {
		a.writeTorrentLog(ctx, id, seedHash, time.Now().UTC(), []DownloadLogLine{{Kind: "x", Summary: "a"}})
		_ = s.Write(ctx, func(tx *sql.Tx) error {
			_, err := tx.Exec("INSERT INTO torrent_seen (downloader_id, hash, seen_at) VALUES (?, ?, ?)", id, seedHash, old.Format(store.TimeLayout))
			return err
		})
	}
	_ = s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO torrent_log (downloader_id, hash, at, kind, summary) VALUES (?, ?, ?, 'x', 'a')",
			up, strings.ToUpper(ownHash), time.Now().UTC().Format(store.TimeLayout))
		return err
	})
	a.clientRows(ctx, a.downloaderNames(ctx), map[clientKey]bool{}, clientListWait)
	n := func(q string, args ...any) int {
		var c int
		_ = s.Read().QueryRow(q, args...).Scan(&c)
		return c
	}
	if a.tlog.lastSweep.IsZero() {
		t.Fatal("one offline Downloader stopped the sweep")
	}
	if c := n("SELECT count(*) FROM torrent_log WHERE downloader_id = ? AND hash = ?", down, seedHash); c != 1 {
		t.Fatal("sweep dropped the offline Downloader's rows")
	}
	if c := n("SELECT count(*) FROM torrent_seen WHERE downloader_id = ?", down); c != 1 {
		t.Fatal("sweep dropped the offline Downloader's seen row")
	}
	if c := n("SELECT count(*) FROM torrent_log WHERE downloader_id = ? AND hash = ?", up, seedHash); c != 0 {
		t.Fatal("sweep kept the healthy Downloader's long-unseen torrent")
	}
	if c := n("SELECT count(*) FROM torrent_log WHERE hash <> lower(hash)"); c != 0 {
		t.Fatal("sweep kept a capitalised-hash row")
	}
}
