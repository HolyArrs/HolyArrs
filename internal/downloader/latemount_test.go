package downloader

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
)

// A late share mount (late-mounts-3, late-mounts-8): the client loses
// sight of the files, or lists nothing for a moment. HolyArrs waits
// instead of failing the download, never deletes data for a client-caused
// failure, and counts a download as vanished only after it is missing for
// missedSyncs syncs in a row.

// missedSyncs is how many syncs in a row a download must be missing from
// its Downloader before it counts as vanished.
const missedSyncs = 3

// missWindow is a step of the test clock long enough that missedSyncs
// syncs, one step apart, span the vanish grace: a download must be missing
// for both a count of syncs and a stretch of time (late-mounts-8).
const missWindow = 10 * time.Minute

// waitingPrefix starts the reason of a download the client can't use the
// files of (a share not mounted yet).
const waitingPrefix = "the Downloader can't use the files"

// failedEngine reports every item as failed by the client itself.
type failedEngine struct{ Engine }

func (f failedEngine) List(ctx context.Context, category string) ([]Job, error) {
	js, err := f.Engine.List(ctx, category)
	for i := range js {
		js[i].State, js[i].Detail = StateFailed, "error"
	}
	return js, err
}

func (f failedEngine) Get(ctx context.Context, id string) (Job, error) {
	j, err := f.Engine.Get(ctx, id)
	j.State, j.Detail = StateFailed, "error"
	return j, err
}

// clientSaysFailed makes the Downloader report every item as failed.
func clientSaysFailed(m *Manager) {
	m.mu.Lock()
	m.engines = nil
	m.mu.Unlock()
	m.Engine = func(d domain.Downloader) (Engine, error) { return failedEngine{NewQbittorrent(d.URL, nil)}, nil }
}

func TestClientErrorStatesBecomePaused(t *testing.T) {
	for _, s := range []string{"error", "missingFiles", "unknown"} {
		if got := qbitState(s); got != StatePaused {
			t.Errorf("qBittorrent %q -> %s, want paused", s, got)
		}
	}
	for _, fin := range []bool{false, true} {
		if got := delugeState("Error", fin); got != StatePaused {
			t.Errorf("Deluge Error (finished %v) -> %s, want paused", fin, got)
		}
	}
	// Transmission error 3 is a local error (files gone): paused. Errors 1
	// and 2 are tracker warnings and errors: the torrent keeps its state.
	local := transmissionTorrent{Hash: hash, Status: 4, Progress: 0.4, Left: 10, Metadata: 1, Error: 3, ErrorString: "No data found! Ensure your drives are connected"}
	if j := local.job("holyarrs"); j.State != StatePaused || !strings.Contains(j.Detail, "error 3") {
		t.Errorf("Transmission local error -> %s (%s), want paused", j.State, j.Detail)
	}
	for _, code := range []int{1, 2} {
		tr := transmissionTorrent{Hash: hash, Status: 4, Progress: 0.4, Left: 10, Metadata: 1, Error: code, ErrorString: "Tracker gave HTTP response code 404"}
		if j := tr.job("holyarrs"); j.State != StateDownloading {
			t.Errorf("Transmission tracker error %d -> %s, want downloading", code, j.State)
		}
	}
}

// missingFiles (a share mounted late) waits as paused with a plain reason:
// the Release is not turned down, nothing is searched again, nothing is
// removed from the client, and it never stalls into failed. It picks up
// again when the files come back.
func TestMissingFilesWaitsAndKeepsData(t *testing.T) {
	w, m := setup(t)
	letChange(t, m) // a removal would reach the fake
	ctx := context.Background()
	id := grabbed(t, w, m)
	w.fake.set(hash, map[string]any{"state": "missingFiles"})
	if n, _ := m.SyncOnce(ctx); n != 1 {
		t.Fatalf("a waiting download must stay active, active %d", n)
	}
	d := w.download(t, id)
	if d.State != "paused" || d.StateReason == nil || !strings.HasPrefix(*d.StateReason, waitingPrefix) || !strings.Contains(*d.StateReason, "is the share mounted?") || !strings.Contains(*d.StateReason, "recheck") {
		t.Fatalf("download %s %v, want paused with %q", d.State, d.StateReason, waitingPrefix)
	}
	if r := w.release(t); r.State == "turned_down" {
		t.Fatalf("release turned down for a client error: %v", r.Verdict)
	}
	if sl := w.slot(t); sl.State != domain.SlotDownloading {
		t.Fatalf("slot %s, want downloading", sl.State)
	}
	if len(w.failedSlots) != 0 || len(w.fake.deleted) != 0 {
		t.Fatalf("searched %v deleted %v", w.failedSlots, w.fake.deleted)
	}
	// Days pass with the share still missing: still waiting, never failed.
	w.clock = w.clock.Add(30 * time.Hour)
	_, _ = m.SyncOnce(ctx)
	if d := w.download(t, id); d.State != "paused" {
		t.Fatalf("after 30 hours waiting: %s", d.State)
	}
	if len(w.fake.deleted) != 0 || len(w.failedSlots) != 0 {
		t.Fatalf("searched %v deleted %v", w.failedSlots, w.fake.deleted)
	}
	// The share is back.
	w.fake.set(hash, map[string]any{"state": "downloading", "progress": 0.3})
	_, _ = m.SyncOnce(ctx)
	if d := w.download(t, id); d.State != "downloading" || d.StateReason != nil {
		t.Fatalf("after the share came back: %s %v", d.State, d.StateReason)
	}
}

// qBittorrent "error" and Deluge-like errors take the same waiting path.
func TestClientErrorWaitsLikeMissingFiles(t *testing.T) {
	w, m := setup(t)
	letChange(t, m)
	id := grabbed(t, w, m)
	w.fake.set(hash, map[string]any{"state": "error"})
	_, _ = m.SyncOnce(context.Background())
	d := w.download(t, id)
	if d.State != "paused" || d.StateReason == nil || !strings.HasPrefix(*d.StateReason, waitingPrefix) {
		t.Fatalf("download %s %v", d.State, d.StateReason)
	}
	if len(w.fake.deleted) != 0 || w.release(t).State == "turned_down" {
		t.Fatalf("deleted %v release %s", w.fake.deleted, w.release(t).State)
	}
}

// A change of reason alone is saved: paused by a person, then the client
// loses the files, is still "paused" but must now say why.
func TestPausedReasonChangeIsSaved(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	id := grabbed(t, w, m)
	w.fake.set(hash, map[string]any{"state": "pausedDL"})
	_, _ = m.SyncOnce(ctx)
	if d := w.download(t, id); d.State != "paused" {
		t.Fatalf("paused by a person: %s", d.State)
	}
	w.fake.set(hash, map[string]any{"state": "missingFiles"})
	_, _ = m.SyncOnce(ctx)
	d := w.download(t, id)
	if d.State != "paused" || d.StateReason == nil || !strings.HasPrefix(*d.StateReason, waitingPrefix) {
		t.Fatalf("reason change not saved: %s %v", d.State, d.StateReason)
	}
}

// Missing once or twice (an empty list while a share mounts, a client
// restart) is not vanished; the third sync in a row is.
func TestVanishedOnlyAfterThreeMissedSyncs(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	id := grabbed(t, w, m)
	w.fake.drop(hash)
	for i := 1; i < missedSyncs; i++ {
		w.advance(missWindow)
		if n, _ := m.SyncOnce(ctx); n != 1 {
			t.Fatalf("missed sync %d: active %d", i, n)
		}
		if d := w.download(t, id); d.State == "vanished" {
			t.Fatalf("vanished after %d missed sync(s)", i)
		}
	}
	if w.release(t).State == "turned_down" || len(w.failedSlots) != 0 {
		t.Fatalf("release %s searched %v before it vanished", w.release(t).State, w.failedSlots)
	}
	w.advance(missWindow)
	_, _ = m.SyncOnce(ctx)
	if d := w.download(t, id); d.State != "vanished" {
		t.Fatalf("want vanished after %d missed syncs, got %s", missedSyncs, d.State)
	}
}

// Missed syncs come every few seconds while a download is active, so a
// client that restarts or waits for its share lists nothing for a minute
// or more: many quick misses are still not vanished. Only once it has
// been missing for the grace time as well does it count as removed.
func TestQuickMissesInsideGraceStayActive(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	id := grabbed(t, w, m)
	w.fake.drop(hash)
	for i := range missedSyncs * 3 {
		w.advance(2 * time.Second)
		if n, _ := m.SyncOnce(ctx); n != 1 {
			t.Fatalf("quick miss %d: active %d", i+1, n)
		}
	}
	if d := w.download(t, id); d.State == "vanished" {
		t.Fatalf("vanished after %d quick misses inside the grace", missedSyncs*3)
	}
	if len(w.failedSlots) != 0 || w.release(t).State == "turned_down" {
		t.Fatalf("searched %v release %s while still inside the grace", w.failedSlots, w.release(t).State)
	}
	w.advance(missWindow)
	_, _ = m.SyncOnce(ctx)
	if d := w.download(t, id); d.State != "vanished" {
		t.Fatalf("want vanished once missing past the grace, got %s", d.State)
	}
}

// Seen again in between starts the count over.
func TestVanishCountStartsOverWhenSeenAgain(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	id := grabbed(t, w, m)
	miss := func(n int) {
		w.fake.drop(hash)
		for range n {
			w.advance(missWindow)
			_, _ = m.SyncOnce(ctx)
		}
	}
	miss(missedSyncs - 1)
	w.fake.set(hash, map[string]any{"name": "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS", "category": "holyarrs", "progress": 0.25, "state": "downloading"})
	_, _ = m.SyncOnce(ctx)
	miss(missedSyncs - 1)
	if d := w.download(t, id); d.State == "vanished" {
		t.Fatal("the missed count did not start over when the download was seen again")
	}
}
