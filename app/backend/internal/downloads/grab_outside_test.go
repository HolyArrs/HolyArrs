package downloads

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// outsideTorrent puts the Release's torrent in the fake qBittorrent outside
// HolyArrs's category, as when it was added by hand in the client.
func (w *world) outsideTorrent(category, state string, progress float64) {
	w.fake.set(hash, map[string]any{"name": "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS.mkv", "category": category, "state": state,
		"progress": progress, "save_path": "/data/downloads", "size": 29588180})
	w.fake.files[hash] = []map[string]any{{"name": "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS.mkv", "size": 29588180, "progress": progress}}
}

// adoptOutside picks the Release whose torrent the client already holds outside
// HolyArrs's category and checks nothing was added again.
func (w *world) adoptOutside(t *testing.T, m *Manager) int64 {
	t.Helper()
	id, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatalf("the torrent already in the client was not taken: %v", err)
	}
	if len(w.fake.added) != 0 {
		t.Fatalf("the torrent was added again: %+v", w.fake.added)
	}
	if d := w.download(t, id); d.ExternalID != hash || w.slot(t).State != mediafiles.SlotDownloading {
		t.Fatalf("download %+v slot %s", d, w.slot(t).State)
	}
	return id
}

func (w *world) wantImportQueued(t *testing.T, id int64) {
	t.Helper()
	if d := w.download(t, id); d.State != "finished" || d.Progress != 1 || len(d.FilesSeen) != 1 {
		t.Fatalf("download %s %.2f files %d, want finished", d.State, d.Progress, len(d.FilesSeen))
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if !slices.Equal(w.finshed, []int64{id}) {
		t.Fatalf("import not queued: OnFinished %v", w.finshed)
	}
}

// A torrent added by hand in qBittorrent and finished there, then picked in
// HolyArrs: the Download follows that torrent and reaches the import.
func TestPickedTorrentFinishedOutsideHolyArrsIsImported(t *testing.T) {
	w, m := setup(t)
	w.outsideTorrent("", "stalledUP", 1)
	id := w.adoptOutside(t, m)
	if _, err := m.SyncOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	w.wantImportQueued(t, id)
}

// The same through the background reader's snapshot, which holds every
// torrent of the client.
func TestPickedTorrentFinishedOutsideHolyArrsIsImportedFromSnapshot(t *testing.T) {
	w, m := setup(t)
	w.outsideTorrent("", "stalledUP", 1)
	id := w.adoptOutside(t, m)
	src := newFakeSource()
	m.Sync = src
	w.advance(time.Second)
	src.set(SyncView{Items: map[string]Job{hash: {ID: hash, Name: "Dune", Category: "", Progress: 1, State: StateFinished, SavePath: "/data/downloads",
		Files: []File{{Name: "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS.mkv", Size: 29588180}}}}, Complete: true, Fresh: true, ReadAt: w.now()})
	if _, err := m.SyncOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	w.wantImportQueued(t, id)
}

// Still downloading outside HolyArrs: the Download shows the client's progress,
// then imports once the client has it all.
func TestPickedTorrentDownloadingOutsideHolyArrsShowsProgress(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	w.outsideTorrent("other", "downloading", 0.4)
	id := w.adoptOutside(t, m)
	if _, err := m.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if d := w.download(t, id); d.State != "downloading" || d.Progress != 0.4 {
		t.Fatalf("download %s %.2f, want downloading 0.40", d.State, d.Progress)
	}
	w.outsideTorrent("other", "uploading", 1)
	if _, err := m.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	w.wantImportQueued(t, id)
}

// The checks that settle unconfirmed sends and lost choices, and the vanish
// timer, leave a Download that follows a torrent from outside HolyArrs alone.
func TestAdoptedTorrentIsNeverSettledOrVanished(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	w.outsideTorrent("", "downloading", 0.4)
	id := w.adoptOutside(t, m)
	for range missedSyncs + 2 {
		w.advance(missWindow)
		w.advance(lostChoiceWait)
		m.settleAttempts(ctx)
		m.settleLostChoices(ctx)
		if _, err := m.SyncOnce(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if d := w.download(t, id); d.State != "downloading" && d.State != "stalled" {
		t.Fatalf("download %s (%v), want still followed", d.State, d.StateReason)
	}
	if sl := w.slot(t); (sl.State != mediafiles.SlotDownloading && sl.State != mediafiles.SlotStalled) || sl.ActiveDownloadID == nil || *sl.ActiveDownloadID != id {
		t.Fatalf("slot %s on %v", sl.State, sl.ActiveDownloadID)
	}
}

// With "Show only HolyArrs items" on, a torrent already in the client in
// another category is not taken. The grab says so at once and the Slot waits
// for a person, instead of a Download that never moves.
func TestPickedTorrentOutsideRefusedWhenOnlyOwnItems(t *testing.T) {
	w, m := setup(t)
	w.exec(t, "UPDATE downloaders SET own_only = 1")
	w.outsideTorrent("tv-sonarr", "stalledUP", 1)
	if _, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System); !errors.Is(err, ErrOutsideTorrent) {
		t.Fatalf("grab: %v, want ErrOutsideTorrent", err)
	}
	if err := m.HandleGrab(context.Background(), grabRun(w, 1)); err != nil {
		t.Fatalf("the refusal should end the grab, not retry: %v", err)
	}
	if len(w.fake.added) != 0 || len(w.fake.steps) != 0 {
		t.Fatalf("the client was changed: adds %+v steps %v", w.fake.added, w.fake.steps)
	}
	if n := w.countDownloads(t); n != 0 {
		t.Fatalf("%d Download rows", n)
	}
	sl := w.slot(t)
	if sl.State != mediafiles.SlotNeedsADecision || sl.LastReason == nil || *sl.LastReason != ErrOutsideTorrent.Error() {
		t.Fatalf("slot %s %v", sl.State, sl.LastReason)
	}
	var summary string
	if err := w.s.ReadEvents().QueryRow("SELECT summary FROM events WHERE kind = ? AND slot_id = ?", mediafiles.EventNeedsDecision, w.slotID).Scan(&summary); err != nil {
		t.Fatalf("no History line: %v", err)
	}
	if st, _ := w.releaseState(t); st == "turned_down" {
		t.Fatal("the Release was turned down; it is fine, only already in the client")
	}
	if !strings.Contains(ErrOutsideTorrent.Error(), "Show only HolyArrs items") {
		t.Fatalf("the reason does not name the setting: %q", ErrOutsideTorrent)
	}
}

// A torrent followed from outside HolyArrs's category is imported, but after
// the import it stays where its owner put it: it is not moved to the
// after-import category nor removed after seeding.
func TestAdoptedTorrentNotMovedOrRemovedAfterImport(t *testing.T) {
	w, m, id, e := movedDownload(t)
	e.jobs[0].Category = "tv-sonarr"
	m.MoveAfterImport(context.Background(), id)
	m.cleanSeeded(context.Background())
	if len(e.moves) != 0 || len(e.removed) != 0 {
		t.Fatalf("another app's torrent was changed: moves %v removed %v", e.moves, e.removed)
	}
	if countEvents(t, w, "download_not_moved", id) != 0 {
		t.Fatal("a History line says the move failed")
	}
	e.jobs[0].Category = "holyarrs"
	m.MoveAfterImport(context.Background(), id)
	if len(e.moves) != 0 {
		t.Fatalf("moved after it was left alone once: %v", e.moves)
	}
}

// A torrent in HolyArrs's own category is taken as before with the setting on.
func TestPickedTorrentInOwnCategoryTakenWhenOnlyOwnItems(t *testing.T) {
	w, m := setup(t)
	w.exec(t, "UPDATE downloaders SET own_only = 1")
	w.outsideTorrent("holyarrs", "stalledUP", 1)
	id := w.adoptOutside(t, m)
	if _, err := m.SyncOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	w.wantImportQueued(t, id)
}
