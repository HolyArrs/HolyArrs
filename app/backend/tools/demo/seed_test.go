package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/files"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/health"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func day(s string) *time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return &t
}

// testShows answers one small show.
type testShows struct{}

func (testShows) SearchShows(context.Context, string) ([]metadata.Show, error) { return nil, nil }

func (testShows) Show(_ context.Context, id string) (metadata.Show, error) {
	if id != "tmdb:1396" {
		return metadata.Show{}, metadata.ErrNotFound
	}
	sh := metadata.Show{Source: "tmdb", Name: "Breaking Bad", Year: 2008, Status: "ended", IDs: map[string]string{"tmdb": "1396"},
		Seasons: []metadata.Season{{Number: 1}, {Number: 2}}}
	for s := 1; s <= 2; s++ {
		for e := 1; e <= 3; e++ {
			sh.Episodes = append(sh.Episodes, metadata.Episode{Season: s, Number: e, Name: "Episode " + string(rune('A'+e)), AirDate: day("2009-01-0" + string(rune('0'+e)))})
		}
	}
	return sh, nil
}

// testWorld is a lab-like config folder: a movie Library folder in
// media/movies, the lab's torrent Source and qBittorrent, and one lab
// video that seed must never touch.
func testWorld(t *testing.T) (options, string) {
	t.Helper()
	root := t.TempDir()
	o := options{ConfigDir: filepath.Join(root, "config"), MediaRoot: filepath.Join(root, "media"), DownloadsDir: filepath.Join(root, "downloads", "complete"), Force: true}
	for _, d := range []string{o.ConfigDir, filepath.Join(o.MediaRoot, "movies", "Old Lab Movie (1950)"), o.DownloadsDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(o.ConfigDir, labMarker), []byte("lab"), 0o644); err != nil {
		t.Fatal(err)
	}
	lab := filepath.Join(o.MediaRoot, "movies", "Old Lab Movie (1950)", "Old Lab Movie (1950).mkv")
	if err := os.WriteFile(lab, []byte("a real lab video"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := database.Open(o.ConfigDir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	err = s.Write(context.Background(), func(tx *sql.Tx) error {
		if _, err := database.Insert(tx, "library_folders", &mediafiles.LibraryFolder{Path: filepath.Join(o.MediaRoot, "movies"), Kind: "movie", Label: "Movies",
			Health: "ok", SameDiskAs: []int64{}}, domain.System); err != nil {
			return err
		}
		if _, err := database.Insert(tx, "library_folders", &mediafiles.LibraryFolder{Path: "C:/Program Files/Git/data/media/movies", Kind: "movie", Label: "Movies",
			Health: "missing", SameDiskAs: []int64{}}, domain.System); err != nil {
			return err
		}
		if _, err := database.Insert(tx, "sources", &domain.Source{Name: "Lab fake indexer", Protocol: "torrent", Format: "torznab", URL: "http://fake-indexer/api",
			UseFor: domain.SourceUse{AutoSearch: true, HandSearch: true}, Kinds: []string{"movie"}, Categories: map[string][]int{"movie": {2000}},
			Priority: 25, State: "enabled", Extra: map[string]string{}, AllowLocal: true}, domain.System); err != nil {
			return err
		}
		_, err := database.Insert(tx, "downloaders", &domain.Downloader{Name: "Lab qBittorrent", Adapter: "qbittorrent", Protocol: "torrent",
			URL: "http://qbittorrent:8090", Category: "holyarrs", Priority: 1, PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok", AllowLocal: true}, domain.System)
		return err
	})
	s.Close()
	if err != nil {
		t.Fatal(err)
	}
	return o, lab
}

var testMovies = &metadata.Static{Movies: []metadata.Movie{
	{Name: "The Matrix", Year: 1999, IDs: map[string]string{"tmdb": "603"}, CinemaDate: day("1999-03-31"), DigitalDate: day("1999-09-21")},
	{Name: "Fight Club", Year: 1999, IDs: map[string]string{"tmdb": "550"}, CinemaDate: day("1999-10-15"), DigitalDate: day("2000-04-25")},
	{Name: "Jurassic Park", Year: 1993, IDs: map[string]string{"tmdb": "329"}, CinemaDate: day("1993-06-11"), DigitalDate: day("1993-10-01")},
	{Name: "Parasite", Year: 2019, IDs: map[string]string{"tmdb": "496243"}, CinemaDate: day("2019-05-30"), DigitalDate: day("2019-10-11")},
	{Name: "The Silence of the Lambs", Year: 1991, IDs: map[string]string{"tmdb": "274"}, CinemaDate: day("1991-02-14"), DigitalDate: day("1991-10-01")},
	{Name: "Terminator 2: Judgment Day", Year: 1991, IDs: map[string]string{"tmdb": "280"}, CinemaDate: day("1991-07-03"), DigitalDate: day("1991-12-01")},
	{Name: "Everything Everywhere All at Once", Year: 2022, IDs: map[string]string{"tmdb": "545611"}, CinemaDate: day("2022-03-24"), DigitalDate: day("2022-05-17")},
	{Name: "Not The Matrix", Year: 2001, IDs: map[string]string{"tmdb": "27205"}},
	{Name: "Avengers: Doomsday", Year: 2099, IDs: map[string]string{"tmdb": "1003596"}, CinemaDate: day("2099-12-18")},
}}

func runSeed(t *testing.T, o options) (*world, string) {
	t.Helper()
	var out bytes.Buffer
	w, err := openWorld(context.Background(), o, providers{Movies: testMovies, Shows: metadata.ShowSources{"tmdb": testShows{}}}, &out)
	if err != nil {
		t.Fatal(err)
	}
	// Windows cannot remove the temp folder while the databases are open.
	t.Cleanup(w.Close)
	if err := w.seed(context.Background()); err != nil {
		w.Close()
		t.Fatalf("seed: %v\n%s", err, out.String())
	}
	return w, out.String()
}

func count(t *testing.T, w *world, q string, args ...any) int {
	t.Helper()
	var n int
	if err := w.s.Read().QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return n
}

func slotState(t *testing.T, w *world, tmdb string) string {
	t.Helper()
	var st string
	if err := w.s.Read().QueryRow(`SELECT s.state FROM slots s JOIN items i ON i.id = s.item_id JOIN titles t ON t.id = i.title_id
		JOIN versions v ON v.id = s.version_id WHERE v.is_main = 1 AND json_extract(t.source_ids, '$.tmdb') = ?`, tmdb).Scan(&st); err != nil {
		t.Fatalf("slot of %s: %v", tmdb, err)
	}
	return st
}

func TestSeedAddsThroughTheAppAndResets(t *testing.T) {
	o, lab := testWorld(t)
	w, out := runSeed(t, o)
	ctx := context.Background()

	// The mangled Windows-path Library folder is gone; show and music
	// folders were added.
	if n := count(t, w, "SELECT count(*) FROM library_folders WHERE path LIKE '%Program Files/Git/%'"); n != 0 {
		t.Fatalf("the unreachable folder is still there\n%s", out)
	}
	if n := count(t, w, "SELECT count(*) FROM library_folders WHERE kind IN ('show', 'music')"); n != 2 {
		t.Fatalf("show and music folders: %d", n)
	}
	// A wrong id is skipped, not added under the wrong name.
	if n := count(t, w, "SELECT count(*) FROM titles WHERE name = 'Not The Matrix'"); n != 0 || !strings.Contains(out, "is \"Not The Matrix\", not \"Inception\"") {
		t.Fatalf("wrong id added or not said\n%s", out)
	}
	want := map[string]string{
		"603":     "in_library",  // adopted at a good copy
		"550":     "in_library",  // adopted low, upgraded by a grab and import
		"329":     "in_library",  // grabbed and imported
		"496243":  "held",        // a Release held until its wait ends
		"274":     "wanted",      // no download is seeded (no fake Review rows)
		"280":     "wanted",      // two Releases turned down by hand
		"545611":  "failed",      // the download failed; a search is queued
		"1003596": "not_out_yet", // upcoming
	}
	for id, st := range want {
		if got := slotState(t, w, id); got != st {
			t.Fatalf("tmdb %s is %s, want %s\n%s", id, got, st, out)
		}
	}
	held, _, err := w.sr.List(ctx, "held", 50, 0)
	if err != nil || len(held) != 1 || held[0].TitleName != "Parasite" || held[0].HoldUntil == nil || !held[0].HoldUntil.After(time.Now()) {
		t.Fatalf("Held list %+v %v", held, err)
	}
	turned, _, err := w.sr.List(ctx, "turned_down", 50, 0)
	// Two turned down by hand, and the Release whose download failed.
	if err != nil || len(turned) != 3 {
		t.Fatalf("Turned down list %+v %v", turned, err)
	}
	if n := count(t, w, "SELECT count(*) FROM files WHERE state = 'replaced'"); n != 1 {
		t.Fatalf("the upgrade replaced %d files", n)
	}
	if n := count(t, w, "SELECT count(*) FROM jobs WHERE kind = 'search_slot' AND state = 'queued'"); n != 1 {
		t.Fatalf("the failed download queued %d searches", n)
	}
	// Episodes: 6 aired, all adopted (complete plan).
	if n := count(t, w, "SELECT count(*) FROM slots s JOIN items i ON i.id = s.item_id WHERE i.kind = 'episode' AND s.state = 'in_library'"); n != 6 {
		t.Fatalf("episodes in the library: %d\n%s", n, out)
	}
	// Nothing is left active for the real Downloader, and History is in the past.
	if n := count(t, w, "SELECT count(*) FROM downloads WHERE state IN ('sent', 'queued', 'downloading', 'stalled', 'paused')"); n != 0 {
		t.Fatalf("%d downloads left active", n)
	}
	var newest string
	_ = w.s.ReadEvents().QueryRow("SELECT max(at) FROM events WHERE kind = 'import_done'").Scan(&newest)
	if newest == "" || newest > database.FormatTime(time.Now().Add(-12*time.Hour)) {
		t.Fatalf("imports are not spread back in time: newest %q", newest)
	}
	// The app's own checks agree: a rescan finds nothing missing, the
	// health checks have no error.
	res, err := w.lib.Rescan(ctx, w.by)
	if err != nil || res.Missing != 0 || res.Unreachable != 0 {
		t.Fatalf("rescan %+v %v", res, err)
	}
	he := &health.Engine{Store: w.s, Version: "test"}
	rs, err := he.Check(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rs {
		if r.State == health.Error {
			t.Fatalf("health error after seed: %+v", r)
		}
	}
	titles := count(t, w, "SELECT count(*) FROM titles")
	rels := count(t, w, "SELECT count(*) FROM releases")
	files := count(t, w, "SELECT count(*) FROM files")
	w.Close()

	// A second run adds nothing.
	w, out = runSeed(t, o)
	if a, b, c := count(t, w, "SELECT count(*) FROM titles"), count(t, w, "SELECT count(*) FROM releases"), count(t, w, "SELECT count(*) FROM files"); a != titles || b != rels || c != files {
		t.Fatalf("second run added: titles %d->%d releases %d->%d files %d->%d\n%s", titles, a, rels, b, files, c, out)
	}

	// Reset removes only what seed added.
	if err := w.reset(ctx); err != nil {
		t.Fatal(err)
	}
	if n := count(t, w, "SELECT count(*) FROM titles WHERE state != 'removed'"); n != 0 {
		t.Fatalf("%d titles left after reset", n)
	}
	if held, _, _ := w.sr.List(ctx, "held", 50, 0); len(held) != 0 {
		t.Fatalf("Held after reset: %d", len(held))
	}
	if n := count(t, w, "SELECT count(*) FROM settings WHERE key = ?", manifestKey); n != 0 {
		t.Fatal("the seed record is still there")
	}
	if b, err := os.ReadFile(lab); err != nil || string(b) != "a real lab video" {
		t.Fatalf("the lab's own video was touched: %v", err)
	}
	_ = filepath.WalkDir(filepath.Dir(o.MediaRoot), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && isPlaceholder(p) {
			t.Errorf("placeholder left after reset: %s", p)
		}
		return nil
	})
	w.Close()
}

func TestSeedRefusesWhileTheAppRunsOrOutsideTheLab(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok\n")) }))
	defer srv.Close()
	if err := checkStopped(context.Background(), srv.URL, true); !errors.Is(err, errRunning) {
		t.Fatalf("running app not refused, even with -force: %v", err)
	}
	if err := checkStopped(context.Background(), "http://127.0.0.1:1", false); err != nil {
		t.Fatalf("stopped app refused: %v", err)
	}
	// Anything but a refused connection is not proof the app is stopped:
	// a name that does not resolve, or an answer cut off.
	cut := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		c, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			c.Close()
		}
	}))
	defer cut.Close()
	for _, u := range []string{"http://holyarrs-demo-test.invalid:18730", cut.URL} {
		if err := checkStopped(context.Background(), u, false); err == nil || errors.Is(err, errRunning) {
			t.Fatalf("%s: unclear state not refused: %v", u, err)
		}
		if err := checkStopped(context.Background(), u, true); err != nil {
			t.Fatalf("%s: -force did not allow an unclear state: %v", u, err)
		}
	}
	// The app's own lock on the config folder refuses, -force or not.
	locked := t.TempDir()
	unlock, err := files.LockDir(locked)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lockConfig(locked); !errors.Is(err, errRunning) {
		t.Fatalf("held lock not refused: %v", err)
	}
	unlock()
	release, err := lockConfig(locked)
	if err != nil {
		t.Fatalf("free folder refused: %v", err)
	}
	release()
	o, _ := testWorld(t)
	o.Force = false
	if err := checkLab(o); err == nil || !strings.Contains(err.Error(), "not the lab") {
		t.Fatalf("a media folder outside the lab was not refused: %v", err)
	}
	// -force waives the media folder check only, never the lab marker.
	o.Force = true
	if err := checkLab(o); err != nil {
		t.Fatalf("forced lab refused: %v", err)
	}
	if err := os.Remove(filepath.Join(o.ConfigDir, labMarker)); err != nil {
		t.Fatal(err)
	}
	if err := checkLab(o); err == nil || !strings.Contains(err.Error(), "-init-lab") {
		t.Fatalf("a config folder without the lab marker was not refused: %v", err)
	}
	// -init-lab marks it; then it passes.
	if err := initLab(o, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := checkLab(o); err != nil {
		t.Fatalf("marked lab refused: %v", err)
	}
	empty := o
	empty.ConfigDir = t.TempDir()
	if err := checkLab(empty); err == nil {
		t.Fatal("an empty config folder was not refused")
	}
	if err := initLab(empty, io.Discard); err == nil {
		t.Fatal("-init-lab marked a folder with no HolyArrs database")
	}
}

func TestSpreadAndNames(t *testing.T) {
	if got := spread(10, 4); len(got) != 4 || got[0] != 0 || got[3] != 9 {
		t.Fatalf("spread %v", got)
	}
	if got := spread(3, 5); len(got) != 3 {
		t.Fatalf("spread %v", got)
	}
	if dotted("WALL·E") != "WALL.E" || dotted("Frieren: Beyond Journey's End") != "Frieren.Beyond.Journeys.End" {
		t.Fatalf("dotted %q %q", dotted("WALL·E"), dotted("Frieren: Beyond Journey's End"))
	}
	if !sameName("The Lord of the Rings: The Fellowship of the Ring", "The Lord of the Rings: The Fellowship") || sameName("Inception", "Interstellar") {
		t.Fatal("sameName")
	}
}

// The History rewrite runs raw SQL on the config folder, so it checks the
// lab marker itself, whoever calls it.
func TestHistoryRewriteNeedsTheLabMarker(t *testing.T) {
	o, _ := testWorld(t)
	if err := os.Remove(filepath.Join(o.ConfigDir, labMarker)); err != nil {
		t.Fatal(err)
	}
	w, err := openWorld(context.Background(), o, providers{}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.Close)
	if err := w.historyTimes(context.Background()); err == nil || !strings.Contains(err.Error(), labMarker) {
		t.Fatalf("History rewritten without the lab marker: %v", err)
	}
}
