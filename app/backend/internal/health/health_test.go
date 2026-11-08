package health

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/updates"
)

type world struct {
	s    *database.Store
	e    *Engine
	lib  string
	free int64
	now  time.Time
	bk   Backups
	bErr error
}

func newWorld(t *testing.T) *world {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	w := &world{s: s, lib: t.TempDir(), free: 50 << 30, now: time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)}
	w.bk = Backups{Newest: w.now.Add(-time.Hour), Every: 24 * time.Hour}
	w.exec(t, "INSERT INTO library_folders (created_at, created_by, updated_at, updated_by, path, kind, label, health, same_disk_as) VALUES ('2026-10-01T00:00:00Z', '{}', '2026-10-01T00:00:00Z', '{}', ?, 'movie', 'Movies', 'ok', '[]')", w.lib)
	w.e = &Engine{Store: s, Version: "v1.1.0", Now: func() time.Time { return w.now },
		FreeSpace: func(string) int64 { return w.free },
		Backups:   func(context.Context) (Backups, error) { return w.bk, w.bErr }}
	return w
}

func (w *world) exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if err := w.s.Write(context.Background(), func(tx *sql.Tx) error { _, err := tx.Exec(q, args...); return err }); err != nil {
		t.Fatal(err)
	}
}

// run runs the checks once and returns them by key.
func (w *world) run(t *testing.T) map[string]domain.HealthCheck {
	t.Helper()
	w.now = w.now.Add(5 * time.Minute)
	cs, err := w.e.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]domain.HealthCheck{}
	for _, c := range cs {
		out[c.Key] = c
	}
	return out
}

func (w *world) events(t *testing.T) []domain.Event {
	t.Helper()
	es, _, err := w.s.Events(context.Background(), database.EventFilter{Kinds: []string{"health_*"}, Limit: 500})
	if err != nil {
		t.Fatal(err)
	}
	return es
}

func (w *world) addDownloader(t *testing.T, name, state string) {
	w.exec(t, `INSERT INTO downloaders (created_at, created_by, updated_at, updated_by, name, adapter, protocol, url, category, priority, path_map, after_import, state, last_error)
		VALUES ('2026-10-01T00:00:00Z', '{}', '2026-10-01T00:00:00Z', '{}', ?, 'qbittorrent', 'torrent', 'http://qb:8080', 'holyarrs', 1, '[]', 'leave', ?, 'connection refused')`, name, state)
}

func TestLibraryFolderCheck(t *testing.T) {
	w := newWorld(t)
	if c := w.run(t)["library_folder:1"]; c.State != OK || c.Kind != "library_folder" {
		t.Fatalf("present folder %+v", c)
	}
	if err := os.Rename(w.lib, w.lib+"-gone"); err != nil {
		t.Fatal(err)
	}
	cs := w.run(t)
	if c := cs["library_folder:1"]; c.State != Error || c.Message == "" {
		t.Fatalf("missing folder %+v", c)
	}
	if _, ok := cs["disk_space:1"]; ok {
		t.Fatal("free space checked on a missing folder")
	}
	_ = os.Rename(w.lib+"-gone", w.lib)
	w.exec(t, "UPDATE library_folders SET health = 'read_only'")
	if c := w.run(t)["library_folder:1"]; c.State != Error {
		t.Fatalf("read-only folder %+v", c)
	}
}

func TestDiskSpaceCheckAndLowSpaceHook(t *testing.T) {
	w := newWorld(t)
	if c := w.run(t)["disk_space:1"]; c.State != OK {
		t.Fatalf("plenty of space %+v", c)
	}
	w.free = 500 << 20
	if c := w.run(t)["disk_space:1"]; c.State != Warning {
		t.Fatalf("under 1 GB %+v", c)
	}
	w.free = 50 << 20
	if c := w.run(t)["disk_space:1"]; c.State != Error {
		t.Fatalf("under the floor %+v", c)
	}
	// An import that needed 4 GB was refused: error until there is room.
	w.free = 3 << 30
	w.e.LowSpace(context.Background(), filepath.Join(w.lib, "Arrival (2016)"), w.free, 4<<30)
	w.e.Wait()
	if c := w.run(t)["disk_space:1"]; c.State != Error {
		t.Fatalf("after a refused import %+v", c)
	}
	w.free = 5 << 30
	if c := w.run(t)["disk_space:1"]; c.State != OK {
		t.Fatalf("room again %+v", c)
	}
	// Unknown free space never warns.
	w.free = -1
	if c := w.run(t)["disk_space:1"]; c.State != OK {
		t.Fatalf("unknown space %+v", c)
	}
}

func TestSourceCheck(t *testing.T) {
	w := newWorld(t)
	w.exec(t, `INSERT INTO sources (created_at, created_by, updated_at, updated_by, name, protocol, format, url, use_for, kinds, categories, limits, priority, state, extra)
		VALUES ('2026-10-01T00:00:00Z', '{}', '2026-10-01T00:00:00Z', '{}', 'Lab', 'torrent', 'torznab', 'http://x/api', '{}', '[]', '{}', '{}', 25, 'enabled', '{}')`)
	if c := w.run(t)["source:1"]; c.State != OK {
		t.Fatalf("working Source %+v", c)
	}
	w.exec(t, "UPDATE sources SET state = 'paused', pause_reason = 'down'")
	if c := w.run(t)["source:1"]; c.State != Warning || c.Subject != "Lab" {
		t.Fatalf("Source down %+v", c)
	}
	w.exec(t, "UPDATE sources SET state = 'disabled', pause_reason = NULL")
	if c := w.run(t)["source:1"]; c.State != OK {
		t.Fatalf("Source paused by a person %+v", c)
	}
}

func TestDownloaderCheck(t *testing.T) {
	w := newWorld(t)
	w.addDownloader(t, "qBittorrent", "ok")
	if c := w.run(t)["downloader:1"]; c.State != OK {
		t.Fatalf("working Downloader %+v", c)
	}
	w.exec(t, "UPDATE downloaders SET state = 'unreachable'")
	if c := w.run(t)["downloader:1"]; c.State != Warning || c.Message != "Downloader qBittorrent does not answer: connection refused" {
		t.Fatalf("unplugged %+v", c)
	}
	w.exec(t, "UPDATE downloaders SET state = 'auth_failed'")
	if c := w.run(t)["downloader:1"]; c.State != Warning {
		t.Fatalf("sign-in turned down %+v", c)
	}
	w.exec(t, "UPDATE downloaders SET state = 'disabled'")
	if c := w.run(t)["downloader:1"]; c.State != OK {
		t.Fatalf("paused by a person %+v", c)
	}
	// Removed: its check goes.
	w.exec(t, "UPDATE downloaders SET removed = 1")
	if _, ok := w.run(t)["downloader:1"]; ok {
		t.Fatal("a removed Downloader is still checked")
	}
}

func TestBackupsCheck(t *testing.T) {
	w := newWorld(t)
	if c := w.run(t)["backups"]; c.State != OK {
		t.Fatalf("fresh backup %+v", c)
	}
	w.bk.Newest = w.now.Add(-4 * 24 * time.Hour)
	if c := w.run(t)["backups"]; c.State != Warning {
		t.Fatalf("old backup %+v", c)
	}
	// Right after start the first backup is still being made: no warning.
	w.bk.Newest = time.Time{}
	if c := w.run(t)["backups"]; c.State != OK {
		t.Fatalf("first backup still to come %+v", c)
	}
	w.bk.Started = true
	if c := w.run(t)["backups"]; c.State != Warning {
		t.Fatalf("no backup %+v", c)
	}
	w.bk.LastErr = "disk full"
	if c := w.run(t)["backups"]; c.State != Error {
		t.Fatalf("failed backup %+v", c)
	}
	w.bk, w.bErr = Backups{}, errors.New("unreadable")
	if c := w.run(t)["backups"]; c.State != Warning {
		t.Fatalf("unreadable folder %+v", c)
	}
}

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b  string
		newer bool
	}{{"v1.2.0", "v1.1.9", true}, {"1.10.0", "1.9.0", true}, {"v1.1.0", "v1.1.0", false}, {"v1.0.0", "v1.1.0", false}, {"dev", "v1.0.0", false}, {"v2.0.0", "dev", false}} {
		if updates.Newer(c.a, c.b) != c.newer {
			t.Errorf("Newer(%s, %s) != %v", c.a, c.b, c.newer)
		}
	}
}

// The flap guard: an Alert-worthy Event only after the new result held for
// two runs in a row, once per change, and nothing for a flap.
func TestBreakAndRecoverEventsWithFlapGuard(t *testing.T) {
	w := newWorld(t)
	w.addDownloader(t, "qBittorrent", "ok")
	w.run(t)
	w.exec(t, "UPDATE downloaders SET state = 'unreachable'")
	w.run(t)
	if n := len(w.events(t)); n != 0 {
		t.Fatalf("%d events after one bad run", n)
	}
	w.run(t)
	es := w.events(t)
	if len(es) != 1 || es[0].Kind != domain.EventHealthBroken || es[0].Data["check"] != "downloader:1" || es[0].Summary == "" {
		t.Fatalf("broken events %+v", es)
	}
	w.run(t)
	w.run(t)
	if n := len(w.events(t)); n != 1 {
		t.Fatalf("de-duplication: %d events", n)
	}
	// A flap (ok once, then bad again) sends nothing.
	w.exec(t, "UPDATE downloaders SET state = 'ok'")
	w.run(t)
	w.exec(t, "UPDATE downloaders SET state = 'unreachable'")
	w.run(t)
	w.run(t)
	if n := len(w.events(t)); n != 1 {
		t.Fatalf("flap sent %d events", n)
	}
	// Fixed for good: one recovered Event.
	w.exec(t, "UPDATE downloaders SET state = 'ok'")
	w.run(t)
	w.run(t)
	w.run(t)
	es = w.events(t)
	if len(es) != 2 || es[0].Kind != domain.EventHealthRecovered {
		t.Fatalf("recovered events %+v", es)
	}
	// The guard is a setting: 1 tells at once.
	w.exec(t, "INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('this_app', 'health_flap_guard', '1', 't', '{}', 't', '{}')")
	w.exec(t, "UPDATE downloaders SET state = 'unreachable'")
	w.run(t)
	if es = w.events(t); len(es) != 3 || es[0].Kind != domain.EventHealthBroken {
		t.Fatalf("guard 1: %+v", es)
	}
}

func TestTrailersCheck(t *testing.T) {
	w := newWorld(t)
	tr := Trailers{}
	w.e.Trailers = func(context.Context) Trailers { return tr }
	if _, ok := w.run(t)["trailers"]; ok {
		t.Fatal("trailers off: no check")
	}
	tr.On = true
	if c := w.run(t)["trailers"]; c.State != OK || c.Subject != "Trailer reader" || c.Message != "" {
		t.Fatalf("no lookup yet %+v", c)
	}
	tr.LastGood = time.Date(2026, 11, 3, 14, 5, 0, 0, time.UTC)
	if c := w.run(t)["trailers"]; c.State != OK || c.Subject != "Trailer reader, last good lookup 3 Nov 14:05 UTC" {
		t.Fatalf("fine %+v", c)
	}
	// Failures below the limit are named but do not warn yet.
	tr.Failed, tr.LastReason = 1, "network"
	if c := w.run(t)["trailers"]; c.State != OK || c.Message != "YouTube could not be reached (the last lookup failed)" {
		t.Fatalf("one failure %+v", c)
	}
	tr.Failed, tr.LastReason = TrailerFailLimit-1, "network"
	if c := w.run(t)["trailers"]; c.State != OK || c.Message != "YouTube could not be reached (4 different videos failed in a row)" {
		t.Fatalf("below the limit %+v", c)
	}
	// A few trailers that will not play (removed, blocked) say nothing yet:
	// that is everyday, not a sign the reader broke.
	for _, reason := range []string{"unavailable", "unplayable"} {
		for _, n := range []int{1, TrailerFailLimit - 1} {
			tr.Failed, tr.LastReason = n, reason
			if c := w.run(t)["trailers"]; c.State != OK || c.Message != "" {
				t.Fatalf("%s x%d: %+v", reason, n, c)
			}
		}
	}
	// Door reasons keep their words; videos that no longer play anywhere
	// say an update may be needed.
	for reason, words := range map[string]string{
		"bot_check":   "YouTube asks to confirm you're not a bot",
		"no_address":  "YouTube changed how its videos are reached; a HolyArrs update will fix it",
		"refused":     "YouTube turned the lookups away",
		"network":     "YouTube could not be reached",
		"unavailable": "Trailers are not being found; a HolyArrs update may be needed",
		"unplayable":  "Trailers are not being found; a HolyArrs update may be needed",
		"":            "Trailer lookups are failing",
	} {
		tr.Failed, tr.LastReason = TrailerFailLimit, reason
		if c := w.run(t)["trailers"]; c.State != Warning || c.Message != words+" (5 different videos failed in a row)" {
			t.Fatalf("%q: %+v", reason, c)
		}
	}
	tr.Failed = 0
	if c := w.run(t)["trailers"]; c.State != OK || c.Message != "" {
		t.Fatalf("recovered %+v", c)
	}
}

// A restart: the first run comes before the trailers check is wired. The
// kept warning stays as it was (not dropped and announced again later);
// once the check reports again it carries on from there.
func TestTrailersRowKeptUntilWired(t *testing.T) {
	w := newWorld(t)
	tr := Trailers{On: true, Failed: TrailerFailLimit, LastReason: "bot_check"}
	w.e.Trailers = func(context.Context) Trailers { return tr }
	for range 3 {
		w.run(t)
	}
	before := len(w.events(t))
	if c := w.run(t)["trailers"]; c.Confirmed != Warning {
		t.Fatalf("set-up %+v", c)
	}
	w.e.Trailers = nil
	if c, ok := w.run(t)["trailers"]; !ok || c.Confirmed != Warning {
		t.Fatalf("kept before wiring: %v %+v", ok, c)
	}
	w.e.Trailers = func(context.Context) Trailers { return tr }
	if c := w.run(t)["trailers"]; c.State != Warning || c.Confirmed != Warning {
		t.Fatalf("after wiring %+v", c)
	}
	if n := len(w.events(t)); n != before {
		t.Fatalf("announced again: %d events, had %d", n, before)
	}
	// Trailer autoplay off still removes it.
	tr.On = false
	if _, ok := w.run(t)["trailers"]; ok {
		t.Fatal("off: no check")
	}
}
