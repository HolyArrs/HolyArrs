package downloads

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

type world struct {
	s       *database.Store
	slotID  int64
	relID   int64
	fake    *fakeQbit
	clock   time.Time
	mu      sync.Mutex
	topics  []string
	finshed []int64
	// failedSlots are the Slots OnFailed was told about; slept holds the grab
	// spacing waits.
	failedSlots []int64
	slept       []time.Duration
}

func (w *world) Publish(topic string, _ any) uint64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.topics = append(w.topics, topic)
	return 0
}

func TestGrabAndSyncToFinished(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	id, err := m.Grab(ctx, w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if len(w.fake.added) != 1 || w.fake.added[0]["category"] != "holyarrs" || w.fake.added[0]["savepath"] != "/data/downloads/complete" {
		t.Fatalf("added %+v", w.fake.added)
	}
	d := w.download(t, id)
	if d.State != "sent" || d.ExternalID != hash || d.ReleaseID == nil || *d.ReleaseID != w.relID {
		t.Fatalf("download %+v", d)
	}
	sl := w.slot(t)
	if sl.State != mediafiles.SlotDownloading || sl.ActiveDownloadID == nil || *sl.ActiveDownloadID != id {
		t.Fatalf("slot %+v", sl)
	}

	w.fake.set(hash, map[string]any{"name": "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS.mkv", "category": "holyarrs", "progress": 0.4,
		"dlspeed": 2048, "eta": 12, "state": "downloading", "save_path": "/data/downloads/complete"})
	w.fake.files[hash] = []map[string]any{{"name": "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS.mkv", "size": 29588180, "progress": 0.4}}
	if n, err := m.SyncOnce(ctx); err != nil || n != 1 {
		t.Fatalf("sync %d %v", n, err)
	}
	d = w.download(t, id)
	if d.State != "downloading" || d.Progress != 0.4 || d.Speed != 2048 || d.ETA == nil || *d.ETA != 12 || d.StartedAt == nil || d.SavePath != "/data/downloads/complete" {
		t.Fatalf("download %+v", d)
	}

	w.fake.set(hash, map[string]any{"progress": 1.0, "state": "stalledUP", "dlspeed": 0})
	if _, err := m.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	d = w.download(t, id)
	if d.State != "finished" || d.Progress != 1 || d.FinishedAt == nil || len(d.FilesSeen) != 1 {
		t.Fatalf("download %+v", d)
	}
	if len(w.finshed) != 1 || w.finshed[0] != id {
		t.Fatalf("OnFinished %v", w.finshed)
	}
	if n, _ := m.SyncOnce(ctx); n != 0 || len(w.finshed) != 1 {
		t.Fatalf("finished download still active (%d) or fired twice", n)
	}
	if len(w.topics) == 0 {
		t.Fatal("nothing published")
	}
}

func TestStalledAfterNoProgressThenBack(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	id, err := m.Grab(ctx, w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	w.fake.set(hash, map[string]any{"name": "x", "category": "holyarrs", "progress": 0.2, "state": "stalledDL"})
	_, _ = m.SyncOnce(ctx)
	w.clock = w.clock.Add(9 * time.Minute)
	_, _ = m.SyncOnce(ctx)
	if d := w.download(t, id); d.State != "downloading" {
		t.Fatalf("stalled too early: %s", d.State)
	}
	w.clock = w.clock.Add(2 * time.Minute)
	_, _ = m.SyncOnce(ctx)
	d := w.download(t, id)
	if d.State != "stalled" || d.StateReason == nil {
		t.Fatalf("want stalled, got %s", d.State)
	}
	if sl := w.slot(t); sl.State != mediafiles.SlotStalled {
		t.Fatalf("slot %s", sl.State)
	}
	w.fake.set(hash, map[string]any{"progress": 0.3, "state": "downloading"})
	_, _ = m.SyncOnce(ctx)
	if d := w.download(t, id); d.State != "downloading" {
		t.Fatalf("want downloading again, got %s", d.State)
	}
	if sl := w.slot(t); sl.State != mediafiles.SlotDownloading {
		t.Fatalf("slot %s", sl.State)
	}
}

func TestVanishedAndFailed(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	id, _ := m.Grab(ctx, w.slotID, w.relID, domain.System)
	w.fake.set(hash, map[string]any{"name": "x", "category": "holyarrs", "progress": 0.1, "state": "downloading"})
	_, _ = m.SyncOnce(ctx)
	w.fake.drop(hash)
	for range missedSyncs {
		w.advance(missWindow)
		_, _ = m.SyncOnce(ctx)
	}
	if d := w.download(t, id); d.State != "vanished" {
		t.Fatalf("want vanished, got %s", d.State)
	}
	if sl := w.slot(t); sl.State != mediafiles.SlotFailed {
		t.Fatalf("slot %s", sl.State)
	}
}

func TestIdleSyncWritesNothing(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	before := w.s.WriteCount()
	for range 5 {
		if n, err := m.SyncOnce(ctx); err != nil || n != 0 {
			t.Fatalf("idle sync %d %v", n, err)
		}
	}
	if after := w.s.WriteCount(); after != before {
		t.Fatalf("idle sync wrote %d times", after-before)
	}
	if m.Interval(0) != 30*time.Second || m.Interval(1) != 2*time.Second {
		t.Fatalf("intervals %v %v", m.Interval(0), m.Interval(1))
	}
}

func TestNotVanishedRightAfterSending(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	w.clock = time.Now().UTC()
	id, _ := m.Grab(ctx, w.slotID, w.relID, domain.System)
	// The Downloader fetches the torrent by URL in the background, so it is not
	// listed yet.
	if n, _ := m.SyncOnce(ctx); n != 1 {
		t.Fatalf("active %d", n)
	}
	if d := w.download(t, id); d.State != "sent" {
		t.Fatalf("want sent during the grace time, got %s", d.State)
	}
	w.clock = w.clock.Add(3 * time.Minute)
	for range missedSyncs {
		w.advance(missWindow)
		_, _ = m.SyncOnce(ctx)
	}
	if d := w.download(t, id); d.State != "vanished" {
		t.Fatalf("want vanished after the grace time, got %s", d.State)
	}
}

func TestGrabRetryAfterKillDoesNotDuplicate(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	// State a kill between the two old writes could leave: Download exists, Slot
	// still chosen.
	var dlID int64
	err := w.s.Write(ctx, func(tx *sql.Tx) error {
		dl := &domain.Download{DownloaderID: 1, ReleaseID: &w.relID, SlotIDs: []int64{w.slotID}, ExternalID: hash, State: "sent",
			FilesSeen: []domain.SeenFile{}, SavePath: m.SavePath, StartedBy: domain.System}
		if _, err := database.Insert(tx, "downloads", dl, domain.System); err != nil {
			return err
		}
		dlID = dl.ID
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	id, err := m.Grab(ctx, w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if id != dlID {
		t.Fatalf("retry made download %d, want the existing %d", id, dlID)
	}
	if len(w.fake.added) != 0 {
		t.Fatalf("retry resent the add: %+v", w.fake.added)
	}
	if n := w.countDownloads(t); n != 1 {
		t.Fatalf("%d downloads", n)
	}
	if sl := w.slot(t); sl.State != mediafiles.SlotDownloading || sl.ActiveDownloadID == nil || *sl.ActiveDownloadID != dlID {
		t.Fatalf("slot %+v", sl)
	}
}

func TestGrabSkipsAddWhenDownloaderAlreadyHasTheHash(t *testing.T) {
	w, m := setup(t)
	w.fake.set(hash, map[string]any{"name": "x", "category": "holyarrs", "progress": 0.1, "state": "downloading"})
	id, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if len(w.fake.added) != 0 {
		t.Fatalf("add resent: %+v", w.fake.added)
	}
	if d := w.download(t, id); d.ExternalID != hash || w.slot(t).State != mediafiles.SlotDownloading {
		t.Fatalf("download %+v", d)
	}
}

func TestGrabUsesTheLinkWithTheKeyPutBack(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	m.Link = func(_ context.Context, rel domain.Release) (string, error) {
		return rel.LinkRef + "?passkey=from-vault", nil
	}
	if _, err := m.Grab(ctx, w.slotID, w.relID, domain.System); err != nil {
		t.Fatal(err)
	}
	if len(w.fake.added) != 1 || w.fake.added[0]["urls"] != "http://fake-indexer/dune-part-two.torrent?passkey=from-vault" {
		t.Fatalf("added %+v", w.fake.added)
	}
	m.Link = func(context.Context, domain.Release) (string, error) { return "", errors.New("key gone") }
	w.fake.added = nil
	if err := w.s.Write(ctx, func(tx *sql.Tx) error { _, err := tx.Exec("DELETE FROM downloads"); return err }); err != nil {
		t.Fatal(err)
	}
	w.fake.mu.Lock()
	w.fake.torrents = map[string]map[string]any{}
	w.fake.mu.Unlock()
	if _, err := m.Grab(ctx, w.slotID, w.relID, domain.System); err == nil || len(w.fake.added) != 0 {
		t.Fatalf("grab without its key: %v %+v", err, w.fake.added)
	}
}

func TestGrabTriesTheNextDownloader(t *testing.T) {
	w, m := setup(t)
	addDeadDownloader(t, w)
	id, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	d := w.download(t, id)
	var name string
	_ = w.s.Read().QueryRow("SELECT name FROM downloaders WHERE id = ?", d.DownloaderID).Scan(&name)
	if name != "Lab qBittorrent" || len(w.fake.added) != 1 {
		t.Fatalf("sent to %q, added %d", name, len(w.fake.added))
	}
	var state string
	_ = w.s.Read().QueryRow("SELECT state FROM downloaders WHERE name = 'Down qBittorrent'").Scan(&state)
	if state != "unreachable" {
		t.Fatalf("dead Downloader state %q", state)
	}
}

func TestGrabStopsAtFirstDownloaderWhenSettingOff(t *testing.T) {
	w, m := setup(t)
	addDeadDownloader(t, w)
	if err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by)
			VALUES ('what_to_pick', 'try_next_downloader', 'false', '', '{}', '', '{}')
			ON CONFLICT (grp, key) DO UPDATE SET value = 'false'`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System); err == nil || !strings.Contains(err.Error(), "Down qBittorrent") {
		t.Fatalf("err %v", err)
	}
	if len(w.fake.added) != 0 {
		t.Fatal("sent to the second Downloader with the setting off")
	}
}

// Choosing a Release again after its Import was rejected starts afresh. The old
// finished Download is not taken as this one.
func TestGrabDoesNotReuseRejectedDownload(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	var oldID int64
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		dl := &domain.Download{DownloaderID: 1, ReleaseID: &w.relID, SlotIDs: []int64{w.slotID}, ExternalID: hash, State: "finished",
			FilesSeen: []domain.SeenFile{}, SavePath: m.SavePath, StartedBy: domain.System}
		if _, err := database.Insert(tx, "downloads", dl, domain.System); err != nil {
			return err
		}
		oldID = dl.ID
		imp := &mediafiles.Import{DownloadID: &dl.ID, FromPath: "/x", Kind: "movie", Plan: []mediafiles.PlanStep{}, Result: []mediafiles.PlaceResult{}, Mode: "auto", State: "rejected"}
		_, err := database.Insert(tx, "imports", imp, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	id, err := m.Grab(ctx, w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if d := w.download(t, id); d.State == "finished" {
		t.Fatalf("reused the finished Download %d (old %d)", id, oldID)
	}
	if w.slot(t).State != mediafiles.SlotDownloading {
		t.Fatal("slot not downloading")
	}
}

func ptr[T any](v T) *T { return &v }

func (w *world) now() time.Time {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.clock
}

// setup makes a movie Slot in state chosen with its chosen Release, and a
// qBittorrent Downloader pointing at a fake.
func setup(t *testing.T) (*world, *Manager) {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	fake, u := startFake(t)
	w := &world{s: s, fake: fake, clock: time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)}
	err = s.Write(context.Background(), func(tx *sql.Tx) error {
		var pr int64
		if err := tx.QueryRow("SELECT id FROM pick_rules LIMIT 1").Scan(&pr); err != nil {
			return err
		}
		lf := &mediafiles.LibraryFolder{Path: "/data/media/movies", Kind: "movie", Label: "Movies", Health: "ok", SameDiskAs: []int64{}}
		ti := &mediafiles.Title{Kind: "movie", GateDriver: "none", Name: "Dune: Part Two", SortName: "dune part two", Year: ptr(2024), Aliases: []string{},
			SourceIDs: map[string]string{}, MetadataSource: "local", NewPartsPolicy: "all", ReleaseGate: "released",
			MusicTypes: []string{}, WhyAdded: "by_hand", Artwork: []string{}, Status: "released", State: "active"}
		for _, x := range []struct {
			table string
			v     any
		}{{"library_folders", lf}, {"titles", ti}} {
			if _, err := database.Insert(tx, x.table, x.v, domain.System); err != nil {
				return err
			}
		}
		it := &mediafiles.Item{TitleID: ti.ID, Kind: "movie"}
		if _, err := database.Insert(tx, "items", it, domain.System); err != nil {
			return err
		}
		v := &mediafiles.Version{TitleID: ti.ID, Label: "HD", PickRulesID: pr, LibraryFolderID: lf.ID, FolderName: "Dune Part Two (2024)", IsMain: true}
		if _, err := database.Insert(tx, "versions", v, domain.System); err != nil {
			return err
		}
		sl := &mediafiles.Slot{ItemID: it.ID, VersionID: v.ID, State: mediafiles.SlotChosen}
		if _, err := database.Insert(tx, "slots", sl, domain.System); err != nil {
			return err
		}
		src := &domain.Source{Name: "Lab", Protocol: "torrent", Format: "torznab", URL: "http://x", Kinds: []string{}, Categories: map[string][]int{}, State: "enabled", Extra: map[string]string{},
			AllowLocal: true} // the lab indexer is on the local network, and qBittorrent fetches its links
		if _, err := database.Insert(tx, "sources", src, domain.System); err != nil {
			return err
		}
		rel := &domain.Release{SourceID: src.ID, Name: "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS", LinkRef: "http://fake-indexer/dune-part-two.torrent",
			Protocol: "torrent", Size: 29588180, InfoHash: ptr(hash), Flags: []string{}, Parsed: map[string]any{}, MatchedSlotIDs: []int64{sl.ID},
			Verdict: map[string]any{}, FoundBy: "auto_search", State: "chosen"}
		if _, err := database.Insert(tx, "releases", rel, domain.System); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE slots SET chosen_release_id = ? WHERE id = ?", rel.ID, sl.ID); err != nil {
			return err
		}
		d := &domain.Downloader{Name: "Lab qBittorrent", Adapter: "qbittorrent", Protocol: "torrent", URL: u, Category: "holyarrs", Priority: 1,
			PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok", AllowLocal: true}
		if _, err := database.Insert(tx, "downloaders", d, domain.System); err != nil {
			return err
		}
		w.slotID, w.relID = sl.ID, rel.ID
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	m := &Manager{Store: s, Live: w, SavePath: "/data/downloads/complete", StallAfter: 10 * time.Minute, ActAfter: 24 * time.Hour, Now: w.now,
		OnFinished: func(_ context.Context, id int64) { w.mu.Lock(); w.finshed = append(w.finshed, id); w.mu.Unlock() },
		OnFailed: func(_ context.Context, id int64) {
			w.mu.Lock()
			w.failedSlots = append(w.failedSlots, id)
			w.mu.Unlock()
		},
		Sleep: func(_ context.Context, d time.Duration) error {
			w.mu.Lock()
			w.slept = append(w.slept, d)
			w.clock = w.clock.Add(d)
			w.mu.Unlock()
			return nil
		}}
	return w, m
}

func (w *world) download(t *testing.T, id int64) domain.Download {
	t.Helper()
	var d domain.Download
	if err := database.Get(context.Background(), w.s.Read(), "downloads", id, &d); err != nil {
		t.Fatal(err)
	}
	return d
}

func (w *world) slot(t *testing.T) mediafiles.Slot {
	t.Helper()
	var s mediafiles.Slot
	if err := database.Get(context.Background(), w.s.Read(), "slots", w.slotID, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func (w *world) countDownloads(t *testing.T) int {
	t.Helper()
	var n int
	if err := w.s.Read().QueryRow("SELECT COUNT(*) FROM downloads").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func addDeadDownloader(t *testing.T, w *world) {
	t.Helper()
	err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		d := &domain.Downloader{Name: "Down qBittorrent", Adapter: "qbittorrent", Protocol: "torrent", URL: "http://127.0.0.1:1", Category: "holyarrs",
			Priority: 0, PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok", AllowLocal: true}
		_, err := database.Insert(tx, "downloaders", d, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}
