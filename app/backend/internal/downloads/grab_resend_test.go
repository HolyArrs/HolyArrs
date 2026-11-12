package downloads

import (
	"context"
	"database/sql"
	"slices"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// resendEngines are the torrent Downloaders whose add refused a torrent an old
// Download row still held.
var resendEngines = []struct {
	name    string
	harness func(*testing.T) conformance
}{{"transmission", transmissionHarness}, {"deluge", delugeHarness}, {"rtorrent", rtorrentHarness}, {"aria2", aria2Harness}}

// resendRig swaps the world's qBittorrent for one of the other torrent
// Downloaders, driven by its fake, and points the Release at that fake's torrent.
func resendRig(t *testing.T, h func(*testing.T) conformance) (*world, *Manager, conformance, int64) {
	t.Helper()
	w, m := setup(t)
	c := h(t)
	w.exec(t, "UPDATE downloaders SET removed = 1")
	did := w.addDownloader(t, &domain.Downloader{Name: "Lab " + c.name, Adapter: c.name, Protocol: "torrent", URL: "http://lab-" + c.name,
		Category: c.release.Category, Priority: 1, AllowLocal: true})
	w.exec(t, "UPDATE releases SET info_hash = ?, link_ref = ? WHERE id = ?", c.release.InfoHash, c.release.URL, w.relID)
	m.Engine = func(domain.Downloader) (Engine, error) { return c.engine, nil }
	return w, m, c, did
}

// oldRow writes a Download that held the torrent for a Slot since removed.
func (w *world) oldRow(t *testing.T, downloader int64, ext, state string) int64 {
	t.Helper()
	dl := &domain.Download{DownloaderID: downloader, SlotIDs: []int64{999}, ExternalID: ext, State: state, Progress: 1,
		FilesSeen: []domain.SeenFile{}, SavePath: "/old", StartedBy: domain.System}
	if err := w.s.Write(context.Background(), func(tx *sql.Tx) error { _, err := database.Insert(tx, "downloads", dl, domain.System); return err }); err != nil {
		t.Fatal(err)
	}
	return dl.ID
}

// followingSlot adds a Slot of a second Version that still follows a Download.
func (w *world) followingSlot(t *testing.T, state mediafiles.SlotState, download int64) {
	t.Helper()
	ctx := context.Background()
	sl := w.slot(t)
	var v mediafiles.Version
	if err := database.Get(ctx, w.s.Read(), "versions", sl.VersionID, &v); err != nil {
		t.Fatal(err)
	}
	v.ID, v.Label, v.IsMain = 0, "4K", false
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := database.Insert(tx, "versions", &v, domain.System); err != nil {
			return err
		}
		_, err := database.Insert(tx, "slots", &mediafiles.Slot{ItemID: sl.ItemID, VersionID: v.ID, State: state, ActiveDownloadID: &download}, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func (w *world) wantResent(t *testing.T, id, old int64) {
	t.Helper()
	if id != old {
		t.Fatalf("the re-send wrote Download %d, not the old row %d", id, old)
	}
	if d := w.download(t, id); d.State != "sent" || d.ReleaseID == nil || *d.ReleaseID != w.relID || !slices.Equal(d.SlotIDs, []int64{w.slotID}) || d.SavePath == "/old" {
		t.Fatalf("old row not started afresh: %s release %v slots %v path %s", d.State, d.ReleaseID, d.SlotIDs, d.SavePath)
	}
	if s := w.slot(t); s.State != mediafiles.SlotDownloading || s.ActiveDownloadID == nil || *s.ActiveDownloadID != id {
		t.Fatalf("slot %s on %v", s.State, s.ActiveDownloadID)
	}
	if n := w.countDownloads(t); n != 1 {
		t.Fatalf("%d Download rows", n)
	}
}

func (w *world) wantUntouched(t *testing.T, old int64, state string) {
	t.Helper()
	if d := w.download(t, old); d.State != state || d.SavePath != "/old" || !slices.Equal(d.SlotIDs, []int64{999}) || d.ReleaseID != nil {
		t.Fatalf("row in use was taken over: %+v", d)
	}
	if s := w.slot(t); s.State != mediafiles.SlotChosen {
		t.Fatalf("slot moved to %s", s.State)
	}
}

func TestResendTakesOverAFinishedOrFailedRow(t *testing.T) {
	for _, e := range resendEngines {
		for _, state := range []string{"finished", "failed"} {
			t.Run(e.name+"/"+state, func(t *testing.T) {
				t.Parallel()
				w, m, c, did := resendRig(t, e.harness)
				old := w.oldRow(t, did, c.release.InfoHash, state)
				id, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System)
				if err != nil {
					t.Fatalf("the same torrent was not sent again: %v", err)
				}
				if n := c.adds(); n != 1 {
					t.Fatalf("%d adds", n)
				}
				w.wantResent(t, id, old)
			})
		}
	}
}

func TestResendAdoptsTheTorrentStillInTheDownloader(t *testing.T) {
	for _, e := range resendEngines {
		t.Run(e.name, func(t *testing.T) {
			t.Parallel()
			w, m, c, did := resendRig(t, e.harness)
			ctx := context.Background()
			if _, err := c.engine.Add(ctx, c.release); err != nil {
				t.Fatal(err)
			}
			old := w.oldRow(t, did, c.release.InfoHash, "finished")
			id, err := m.Grab(ctx, w.slotID, w.relID, domain.System)
			if err != nil {
				t.Fatalf("the seeding torrent was not taken again: %v", err)
			}
			if n := c.adds(); n != 1 {
				t.Fatalf("the torrent was added again: %d adds", n)
			}
			w.wantResent(t, id, old)
		})
	}
}

func TestResendNeverTakesARowStillInUse(t *testing.T) {
	cases := []struct {
		name, state string
		setup       func(t *testing.T, w *world, old int64)
	}{
		{"active", "downloading", func(*testing.T, *world, int64) {}},
		{"importing", "finished", func(t *testing.T, w *world, old int64) { w.followingSlot(t, mediafiles.SlotImporting, old) }},
		{"waiting on a decision", "failed", func(t *testing.T, w *world, old int64) { w.followingSlot(t, mediafiles.SlotNeedsADecision, old) }},
		{"removal waiting", "cleaned", func(t *testing.T, w *world, old int64) {
			w.exec(t, "UPDATE downloads SET removal_pending = 1 WHERE id = ?", old)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name+"/transmission", func(t *testing.T) {
			t.Parallel()
			w, m, c, did := resendRig(t, transmissionHarness)
			old := w.oldRow(t, did, c.release.InfoHash, tc.state)
			tc.setup(t, w, old)
			if _, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System); err == nil {
				t.Fatal("the re-send took a row still in use")
			}
			if n := c.adds(); n != 0 {
				t.Fatalf("%d adds", n)
			}
			w.wantUntouched(t, old, tc.state)
		})
		// qBittorrent's add never looks at other rows, so the send must refuse first.
		t.Run(tc.name+"/qbittorrent", func(t *testing.T) {
			t.Parallel()
			w, m := setup(t)
			old := w.oldRow(t, 1, hash, tc.state)
			tc.setup(t, w, old)
			if _, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System); err == nil {
				t.Fatal("the re-send took a row still in use")
			}
			if len(w.fake.added) != 0 {
				t.Fatalf("sent to qBittorrent anyway: %d adds", len(w.fake.added))
			}
			w.wantUntouched(t, old, tc.state)
		})
	}
}

// The write that records a send checks the row again, since a Downloader can
// answer with an id the send could not know beforehand.
func TestHeldByKeepsRowsInUse(t *testing.T) {
	w, _ := setup(t)
	done := w.oldRow(t, 1, "aaa", "finished")
	active := w.oldRow(t, 1, "bbb", "downloading")
	followed := w.oldRow(t, 1, "ccc", "finished")
	w.followingSlot(t, mediafiles.SlotImporting, followed)
	removing := w.oldRow(t, 1, "ddd", "cleaned")
	w.exec(t, "UPDATE downloads SET removal_pending = 0 WHERE id = ?", removing)
	for _, tc := range []struct {
		ext   string
		id    int64
		inUse bool
	}{{"aaa", done, false}, {"bbb", active, true}, {"ccc", followed, true}, {"ddd", removing, true}, {"zzz", 0, false}} {
		if err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
			id, inUse, err := heldBy(context.Background(), tx, 1, tc.ext)
			if err == nil && (id != tc.id || inUse != tc.inUse) {
				t.Errorf("%s: row %d in use %v, want %d %v", tc.ext, id, inUse, tc.id, tc.inUse)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestResendNeverTakesAnotherAppsTorrent(t *testing.T) {
	for _, e := range resendEngines {
		t.Run(e.name, func(t *testing.T) {
			t.Parallel()
			w, m, c, did := resendRig(t, e.harness)
			ctx := context.Background()
			theirs := c.release
			theirs.Category = "sonarr"
			if _, err := c.engine.Add(ctx, theirs); err != nil {
				t.Fatal(err)
			}
			old := w.oldRow(t, did, c.release.InfoHash, "finished")
			if _, err := m.Grab(ctx, w.slotID, w.relID, domain.System); err == nil {
				t.Fatal("took a torrent in another category")
			}
			if n := c.adds(); n != 1 {
				t.Fatalf("%d adds", n)
			}
			w.wantUntouched(t, old, "finished")
		})
	}
}
