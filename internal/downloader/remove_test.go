package downloader

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/domain"
)

var luis = domain.Actor{Kind: "user", ID: 1}

// Remove with the defaults (Radarr's): out of the Downloader with its files,
// the Release is not turned down, nothing is searched, the Slot is wanted.
func TestRemoveKeepsTheReleaseByDefault(t *testing.T) {
	w, m := setup(t)
	letChange(t, m) // it removes an item already in the Downloader
	id := grabbed(t, w, m)
	if err := m.Remove(context.Background(), id, RemoveChoice{DeleteFiles: true}, luis, "luis"); err != nil {
		t.Fatal(err)
	}
	d := w.download(t, id)
	if d.State != "cleaned" || d.StateReason == nil || *d.StateReason != "removed by luis" {
		t.Fatalf("download %s %v", d.State, d.StateReason)
	}
	if len(w.fake.deleted) != 1 || w.fake.deleted[0] != hash+"|true" {
		t.Fatalf("deleted %v", w.fake.deleted)
	}
	if r := w.release(t); r.State == "turned_down" {
		t.Fatal("turned down without asking")
	}
	if sl := w.slot(t); sl.State != domain.SlotWanted {
		t.Fatalf("slot %s", sl.State)
	}
	if len(w.failedSlots) != 0 {
		t.Fatal("searched without turning down")
	}
	if err := m.Remove(context.Background(), id, RemoveChoice{}, luis, "luis"); !errors.Is(err, ErrNotActive) {
		t.Fatalf("second remove: %v", err)
	}
}

// Turn down and search again: the Release is turned down and the Slot
// searched; files kept in the Downloader when asked.
func TestRemoveTurnDownAndSearchAgain(t *testing.T) {
	w, m := setup(t)
	letChange(t, m) // it removes an item already in the Downloader
	id := grabbed(t, w, m)
	if err := m.Remove(context.Background(), id, RemoveChoice{DeleteFiles: false, TurnDown: true, SearchAgain: true}, luis, "luis"); err != nil {
		t.Fatal(err)
	}
	if len(w.fake.deleted) != 1 || w.fake.deleted[0] != hash+"|false" {
		t.Fatalf("deleted %v", w.fake.deleted)
	}
	if r := w.release(t); r.State != "turned_down" || !strings.Contains(r.Verdict["turned_down_reason"].(string), "removed by luis") {
		t.Fatalf("release %s %v", r.State, r.Verdict)
	}
	if sl := w.slot(t); sl.State != domain.SlotFailed {
		t.Fatalf("slot %s", sl.State)
	}
	if len(w.failedSlots) != 1 || w.failedSlots[0] != w.slotID {
		t.Fatalf("searched %v", w.failedSlots)
	}
}

// Turn down without searching: the Slot rests (wanted).
func TestRemoveTurnDownNoSearch(t *testing.T) {
	w, m := setup(t)
	id := grabbed(t, w, m)
	if err := m.Remove(context.Background(), id, RemoveChoice{DeleteFiles: true, TurnDown: true}, luis, ""); err != nil {
		t.Fatal(err)
	}
	if r := w.release(t); r.State != "turned_down" {
		t.Fatalf("release %s", r.State)
	}
	if sl := w.slot(t); sl.State != domain.SlotWanted || len(w.failedSlots) != 0 {
		t.Fatalf("slot %s searched %v", sl.State, w.failedSlots)
	}
}

// A Downloader that already lost it is fine; the row is still removed.
func TestRemoveGoneFromTheDownloader(t *testing.T) {
	w, m := setup(t)
	id := grabbed(t, w, m)
	w.fake.mu.Lock()
	delete(w.fake.torrents, hash)
	w.fake.mu.Unlock()
	if err := m.Remove(context.Background(), id, RemoveChoice{DeleteFiles: true}, luis, "luis"); err != nil {
		t.Fatal(err)
	}
	if d := w.download(t, id); d.State != "cleaned" {
		t.Fatalf("download %s", d.State)
	}
}

// 4.14 check (b): a finished download (an album imported track by track,
// its Library copies hardlinked to its files) is not active: Remove and
// Mark failed refuse it, so they never delete its files.
func TestRemoveAndMarkFailedLeaveFinishedDownloads(t *testing.T) {
	w, m := setup(t)
	id := grabbed(t, w, m)
	if err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE downloads SET state = 'finished' WHERE id = ?", id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := m.Remove(context.Background(), id, RemoveChoice{DeleteFiles: true}, luis, "luis"); !errors.Is(err, ErrNotActive) {
		t.Fatalf("remove: %v", err)
	}
	if err := m.MarkFailed(context.Background(), id, luis, "luis"); !errors.Is(err, ErrNotActive) {
		t.Fatalf("mark failed: %v", err)
	}
	if len(w.fake.deleted) != 0 {
		t.Fatalf("files deleted: %v", w.fake.deleted)
	}
}

// With "HolyArrs can change this Downloader" off, a grab is still sent but a
// Remove never reaches the client.
func TestSwitchOffSendsButNeverRemoves(t *testing.T) {
	w, m := setup(t)
	id := grabbed(t, w, m)
	_ = m.Remove(context.Background(), id, RemoveChoice{DeleteFiles: true}, luis, "luis")
	if len(w.fake.deleted) != 0 {
		t.Fatalf("deleted %v with the switch off", w.fake.deleted)
	}
}
