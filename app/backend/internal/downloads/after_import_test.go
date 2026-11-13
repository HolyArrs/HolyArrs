package downloads

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// moveEngine is a seedEngine that can also move a torrent to a category.
type moveEngine struct {
	*seedEngine
	moves []string
	err   error
}

func (e *moveEngine) SetLocation(context.Context, []string, string) error { return nil }
func (e *moveEngine) SetTorrentLimits(context.Context, []string, int64, int64) error {
	return nil
}
func (e *moveEngine) SetCategory(_ context.Context, id, cat string) error {
	if e.err != nil {
		return e.err
	}
	e.moves = append(e.moves, id+"|"+cat)
	for i := range e.jobs {
		if e.jobs[i].ID == id {
			e.jobs[i].Category = cat
		}
	}
	return nil
}

// movedDownload is an imported qBittorrent torrent on a Downloader set to
// move it to "holyarrs-done" after import.
func movedDownload(t *testing.T) (*world, *Manager, int64, *moveEngine) {
	t.Helper()
	w, m, id, eng := seededDownload(t, "qbittorrent", `{}`, Job{Detail: "uploading"})
	if err := execSQL(w, `UPDATE downloaders SET after_import = 'leave', category_after_import = 'holyarrs-done'`); err != nil {
		t.Fatal(err)
	}
	me := &moveEngine{seedEngine: eng}
	m.Engine = func(domain.Downloader) (Engine, error) { return me, nil }
	m.Forget(w.download(t, id).DownloaderID)
	return w, m, id, me
}

func TestMoveAfterImportMovesOnceWithHistory(t *testing.T) {
	w, m, id, e := movedDownload(t)
	m.MoveAfterImport(context.Background(), id)
	if len(e.moves) != 1 || e.moves[0] != hash+"|holyarrs-done" {
		t.Fatalf("moves %v", e.moves)
	}
	if countEvents(t, w, "download_changed", id) != 1 {
		t.Fatal("no History line for the move")
	}
	// A pack imported over more runs calls the hook again: it moves once.
	m.MoveAfterImport(context.Background(), id)
	if len(e.moves) != 1 || countEvents(t, w, "download_changed", id) != 1 {
		t.Fatalf("moved again: %v", e.moves)
	}
}

func TestMoveAfterImportOffByDefault(t *testing.T) {
	w, m, id, e := movedDownload(t)
	if err := execSQL(w, `UPDATE downloaders SET category_after_import = ''`); err != nil {
		t.Fatal(err)
	}
	m.MoveAfterImport(context.Background(), id)
	if len(e.moves) != 0 {
		t.Fatalf("moves %v", e.moves)
	}
}

func TestMoveAfterImportSkipsReadOnlyPausedRemoved(t *testing.T) {
	for _, q := range []string{`UPDATE downloaders SET writable = 0`, `UPDATE downloaders SET state = 'disabled'`, `UPDATE downloaders SET removed = 1`} {
		w, m, id, e := movedDownload(t)
		if err := execSQL(w, q); err != nil {
			t.Fatal(err)
		}
		m.Forget(w.download(t, id).DownloaderID)
		m.MoveAfterImport(context.Background(), id)
		if len(e.moves) != 0 {
			t.Fatalf("%s: moves %v", q, e.moves)
		}
	}
}

func TestMoveAfterImportWaitsForOpenPack(t *testing.T) {
	w, m, id, e := movedDownload(t)
	if err := execSQL(w, `UPDATE downloads SET pack_files = '[{"i":0,"name":"a.mkv"}]', files_seen = '[{"name":"a.mkv","size":1,"priority":1},{"name":"b.mkv","size":1,"priority":0}]'`); err != nil {
		t.Fatal(err)
	}
	m.MoveAfterImport(context.Background(), id)
	if len(e.moves) != 0 {
		t.Fatalf("moved an open pack: %v", e.moves)
	}
	if err := execSQL(w, `UPDATE downloads SET files_seen = '[{"name":"a.mkv","size":1,"priority":1},{"name":"b.mkv","size":1,"priority":1}]'`); err != nil {
		t.Fatal(err)
	}
	m.MoveAfterImport(context.Background(), id)
	if len(e.moves) != 1 {
		t.Fatalf("moves %v", e.moves)
	}
}

func TestMoveAfterImportGoneTorrentIsQuiet(t *testing.T) {
	w, m, id, e := movedDownload(t)
	e.err = ErrNotFound
	m.MoveAfterImport(context.Background(), id)
	if countEvents(t, w, "download_not_moved", id) != 0 || countEvents(t, w, "download_changed", id) != 0 {
		t.Fatal("a gone torrent wrote a History line")
	}
	if moved, _ := m.categoryMoved(context.Background(), id); !moved {
		t.Fatal("a gone torrent is tried again forever")
	}
}

func TestMoveAfterImportFailureTriesAgain(t *testing.T) {
	w, m, id, e := movedDownload(t)
	e.err = errors.New("the Downloader answered HTTP 500")
	m.MoveAfterImport(context.Background(), id)
	if countEvents(t, w, "download_not_moved", id) != 1 {
		t.Fatal("no warning line")
	}
	e.err = nil
	m.MoveAfterImport(context.Background(), id)
	if len(e.moves) != 1 {
		t.Fatalf("moves %v", e.moves)
	}
}

// After the move the seeding clean-up still finds the torrent in its new
// category and removes it at its goal.
func TestSeedCleanupFindsMovedTorrent(t *testing.T) {
	w, m, id, e := movedDownload(t)
	m.MoveAfterImport(context.Background(), id)
	e.jobs[0].Detail = "stoppedUP"
	if err := execSQL(w, `UPDATE downloaders SET after_import = 'remove'`); err != nil {
		t.Fatal(err)
	}
	m.cleanSeeded(context.Background())
	if len(e.removed) != 1 || e.removed[0] != hash+"|keep" {
		t.Fatalf("removed %v", e.removed)
	}
}

func TestQbitSetCategoryMakesCategoryFirst(t *testing.T) {
	f := newFake()
	f.set(hash, map[string]any{"category": "holyarrs"})
	srv := httptest.NewServer(f)
	defer srv.Close()
	q := NewQbittorrent(srv.URL, nil)
	if err := q.SetCategory(context.Background(), hash, "holyarrs-done"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(f.steps, ",") != "setCategory "+hash+" holyarrs-done" || strings.Join(f.cats, ",") != "holyarrs-done" {
		t.Fatalf("steps %v cats %v", f.steps, f.cats)
	}
	if err := q.SetCategory(context.Background(), strings.Repeat("a", 40), "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("gone torrent: %v", err)
	}
}

// Pause all and Resume all touch only this Downloader's torrents on a client
// another app shares.
func TestQbitPauseAllLeavesOtherApps(t *testing.T) {
	f := newFake()
	other := strings.Repeat("b", 40)
	f.set(hash, map[string]any{"category": "holyarrs"})
	f.set(other, map[string]any{"category": "tv-sonarr"})
	srv := httptest.NewServer(f)
	defer srv.Close()
	q := NewQbittorrent(srv.URL, nil)
	q.cats = []string{"holyarrs", "holyarrs-done"}
	if err := q.PauseAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := q.ResumeAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(f.steps, ",")
	if got != "stop "+hash+",start "+hash || strings.Contains(got, other) || strings.Contains(got, "all") {
		t.Fatalf("steps %v", f.steps)
	}
}

func TestAfterImportCategorySettings(t *testing.T) {
	d := domain.Downloader{Adapter: "qbittorrent", Category: "holyarrs", CategoryShows: "tv"}
	for _, c := range []struct {
		adapter, cat string
		want         error
	}{
		{"qbittorrent", "holyarrs-done", nil},
		{"qbittorrent", "", nil},
		{"qbittorrent", "HolyArrs", ErrAfterImportSame},
		{"qbittorrent", "tv", ErrAfterImportSame},
		{"transmission", "done", ErrAfterImportCategory},
		{"deluge", "done", ErrAfterImportCategory},
		{"sabnzbd", "done", ErrAfterImportCategory},
	} {
		x := d
		x.Adapter = c.adapter
		cat := c.cat
		err := fill(Input{Name: "q", CategoryAfterImport: &cat}, &x, false)
		if c.want == nil && err != nil || c.want != nil && !errors.Is(err, c.want) {
			t.Fatalf("%s %q: %v", c.adapter, c.cat, err)
		}
		if err == nil && x.CategoryAfterImport != c.cat {
			t.Fatalf("stored %q", x.CategoryAfterImport)
		}
	}
	x := d
	x.CategoryAfterImport = "holyarrs-done"
	if got := strings.Join(x.Categories(), ","); got != "holyarrs,tv,holyarrs-done" {
		t.Fatalf("categories %s", got)
	}
}
