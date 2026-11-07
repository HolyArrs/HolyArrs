package downloader

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
)

// grabRun is the grab Job for the world's Slot at the given try of five.
func grabRun(w *world, attempt int) *jobs.Run {
	return &jobs.Run{Job: domain.Job{Subject: &domain.Subject{Type: "slot", ID: w.slotID}, Params: map[string]any{"release_id": float64(w.relID)}, Attempt: attempt, MaxAttempts: 5}}
}

func (w *world) exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(q, args...)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func (w *world) releaseState(t *testing.T) (state, reason string) {
	t.Helper()
	var r sql.NullString
	if err := w.s.Read().QueryRow("SELECT state, json_extract(verdict, '$.turned_down_reason') FROM releases WHERE id = ?", w.relID).Scan(&state, &r); err != nil {
		t.Fatal(err)
	}
	return state, r.String
}

// The same Release chosen again for a Slot whose earlier Download finished
// and was imported is sent again, not left on "Waiting to import".
func TestGrabDoesNotReuseImportedDownload(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		dl := &domain.Download{DownloaderID: 1, ReleaseID: &w.relID, SlotIDs: []int64{w.slotID}, ExternalID: hash, State: "finished",
			FilesSeen: []domain.SeenFile{}, SavePath: m.SavePath, StartedBy: domain.System}
		if _, err := database.Insert(tx, "downloads", dl, domain.System); err != nil {
			return err
		}
		imp := &domain.Import{DownloadID: &dl.ID, FromPath: "/x", Kind: "movie", Plan: []domain.PlanStep{}, Result: []domain.PlaceResult{}, Mode: "auto", State: "done"}
		_, err := database.Insert(tx, "imports", imp, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	id, err := m.Grab(ctx, w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if len(w.fake.added) != 1 {
		t.Fatalf("the torrent was not sent again: %d adds", len(w.fake.added))
	}
	if d := w.download(t, id); d.State != "sent" || !slices.Equal(d.SlotIDs, []int64{w.slotID}) {
		t.Fatalf("download %d is %s for %v", id, d.State, d.SlotIDs)
	}
	if s := w.slot(t); s.State != domain.SlotDownloading || s.ActiveDownloadID == nil || *s.ActiveDownloadID != id {
		t.Fatalf("slot %s on %v", s.State, s.ActiveDownloadID)
	}
	// The client finishes it again: the import is started for it.
	w.fake.set(hash, map[string]any{"name": "Dune", "state": "uploading", "progress": 1.0, "category": "holyarrs", "save_path": "/data/downloads/complete"})
	if _, err := m.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if d := w.download(t, id); d.State != "finished" || !slices.Contains(w.finshed, id) {
		t.Fatalf("download %s, finished calls %v", d.State, w.finshed)
	}
}

// A torrent Release with no info hash whose link has no file behind it (the
// #99 decoy: the Source answers 404) is never handed to qBittorrent: the
// grab ends at once, the Release is turned down and the Slot is searched again.
func TestGrabHashlessReleaseFreesTheSlot(t *testing.T) {
	w, m := setup(t)
	gone := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(gone.Close)
	w.exec(t, "UPDATE releases SET info_hash = NULL, link_ref = ?", gone.URL+"/decoy.torrent")
	w.exec(t, "UPDATE sources SET allow_local_network = 1, url = ?", gone.URL+"/api")
	if err := m.HandleGrab(context.Background(), grabRun(w, 1)); err != nil {
		t.Fatalf("the Job should end, not retry: %v", err)
	}
	if len(w.fake.added) != 0 {
		t.Fatalf("sent to qBittorrent anyway: %v", w.fake.added)
	}
	s := w.slot(t)
	if s.State != domain.SlotWanted || s.ChosenReleaseID != nil || s.LastReason == nil || !strings.Contains(*s.LastReason, "no such Release file") {
		t.Fatalf("slot %s, chosen %v, reason %v", s.State, s.ChosenReleaseID, s.LastReason)
	}
	if st, why := w.releaseState(t); st != "turned_down" || !strings.HasPrefix(why, "could not be sent: ") {
		t.Fatalf("release %s (%q)", st, why)
	}
	if !slices.Equal(w.failedSlots, []int64{w.slotID}) {
		t.Fatalf("searched again: %v", w.failedSlots)
	}
}

// A link to a .torrent file with no info hash from a Source at home (a
// private site through a Jackett or Prowlarr on the local network, #47):
// HolyArrs fetches the file, reads its hash and uploads it, and follows the
// download by that hash. A Source that turns the key down for now (401) is
// tried again, not turned down.
func TestGrabHashlessLocalLinkUploadsTheFile(t *testing.T) {
	tor := []byte("d8:announce14:http://t/a/ann4:infod6:lengthi3e4:name5:dune.12:piece lengthi16384e6:pieces20:aaaaaaaaaaaaaaaaaaaaee")
	want, err := torrentFileHash(tor)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("uploaded", func(t *testing.T) {
		w, m := setup(t)
		src := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) { _, _ = rw.Write(tor) }))
		t.Cleanup(src.Close)
		w.exec(t, "UPDATE releases SET info_hash = NULL, link_ref = ?", src.URL+"/dl/site/?path=x")
		w.exec(t, "UPDATE sources SET allow_local_network = 1, url = ?", src.URL+"/api/v2.0/indexers/site/results/torznab/")
		id, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System)
		if err != nil {
			t.Fatal(err)
		}
		if len(w.fake.added) != 1 || w.fake.added[0]["torrent"] != string(tor) || w.fake.added[0]["category"] != "holyarrs" {
			t.Fatalf("added %+v, want the file", w.fake.added)
		}
		if d := w.download(t, id); d.ExternalID != want {
			t.Fatalf("followed as %q, want %s", d.ExternalID, want)
		}
	})
	t.Run("home tracker kept", func(t *testing.T) {
		// A Source allowed at home keeps a tracker at home in the file (the
		// cleaning is for Sources that may not reach the local network).
		home := []byte("d8:announce32:http://192.168.1.5:9000/announce4:infod6:lengthi3e4:name5:dune.12:piece lengthi16384e6:pieces20:aaaaaaaaaaaaaaaaaaaaee")
		w, m := setup(t)
		src := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) { _, _ = rw.Write(home) }))
		t.Cleanup(src.Close)
		w.exec(t, "UPDATE releases SET info_hash = NULL, link_ref = ?", src.URL+"/dl/site/?path=x")
		w.exec(t, "UPDATE sources SET allow_local_network = 1, url = ?", src.URL+"/api")
		if _, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System); err != nil {
			t.Fatal(err)
		}
		if len(w.fake.added) != 1 || w.fake.added[0]["torrent"] != string(home) {
			t.Fatalf("added %+v, want the file as it was", w.fake.added)
		}
	})
	t.Run("key turned down", func(t *testing.T) {
		w, m := setup(t)
		src := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) { rw.WriteHeader(http.StatusUnauthorized) }))
		t.Cleanup(src.Close)
		w.exec(t, "UPDATE releases SET info_hash = NULL, link_ref = ?", src.URL+"/dl/site/?path=x")
		w.exec(t, "UPDATE sources SET allow_local_network = 1, url = ?", src.URL+"/api/v2.0/indexers/site/results/torznab/")
		if err := m.HandleGrab(context.Background(), grabRun(w, 1)); err == nil {
			t.Fatal("a 401 should be tried again")
		}
		if st, _ := w.releaseState(t); st != "chosen" || w.slot(t).State != domain.SlotChosen || len(w.fake.added) != 0 {
			t.Fatalf("release %s, slot %s, added %v", st, w.slot(t).State, w.fake.added)
		}
	})
}

// A home Source may only be reached at home itself: its Release link to
// another local address is never fetched, so a link from a site cannot make
// HolyArrs call a device at home.
func TestGrabHashlessLocalLinkOnlyTheSourceItself(t *testing.T) {
	w, m := setup(t)
	hit := 0
	dev := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) { hit++ }))
	t.Cleanup(dev.Close)
	w.exec(t, "UPDATE releases SET info_hash = NULL, link_ref = ?", dev.URL+"/router/admin")
	w.exec(t, "UPDATE sources SET allow_local_network = 1, url = 'http://jackett.lan:9117/api'")
	if _, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System); err == nil {
		t.Fatal("a link to another local address was sent")
	}
	if hit != 0 || len(w.fake.added) != 0 {
		t.Fatalf("the device was called %d times, added %v", hit, w.fake.added)
	}
	// The Source's own host on another port is another service: refused too.
	u, _ := url.Parse(dev.URL)
	w.exec(t, "UPDATE sources SET url = ?", "http://"+u.Hostname()+":1/api")
	if _, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System); err == nil || hit != 0 {
		t.Fatalf("another port of the Source's host was reached (%v, %d calls)", err, hit)
	}
}

// A magnet link carries the info hash itself.
func TestGrabMagnetWithoutInfoHash(t *testing.T) {
	w, m := setup(t)
	w.exec(t, "UPDATE releases SET info_hash = NULL, link_ref = ?", "magnet:?xt=urn:btih:"+strings.ToUpper(hash)+"&dn=Dune")
	id, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if d := w.download(t, id); d.ExternalID != hash {
		t.Fatalf("followed as %q", d.ExternalID)
	}
}

// A Downloader that does not answer keeps the Slot chosen while tries are
// left; after the last one the Slot goes back to wanted and the Release
// stays as it was (the Downloader, not the Release, was at fault).
func TestGrabLastTryFreesTheSlot(t *testing.T) {
	w, m := setup(t)
	w.exec(t, "UPDATE downloaders SET url = 'http://127.0.0.1:1'")
	ctx := context.Background()
	if err := m.HandleGrab(ctx, grabRun(w, 1)); err == nil {
		t.Fatal("an unanswered send should be tried again")
	}
	if s := w.slot(t); s.State != domain.SlotChosen {
		t.Fatalf("slot %s after the first try", s.State)
	}
	if err := m.HandleGrab(ctx, grabRun(w, 5)); err == nil {
		t.Fatal("the last try still fails the Job")
	}
	if s := w.slot(t); s.State != domain.SlotWanted || s.ChosenReleaseID != nil {
		t.Fatalf("slot %s, chosen %v after the last try", s.State, s.ChosenReleaseID)
	}
	if st, _ := w.releaseState(t); st != "chosen" {
		t.Fatalf("release %s", st)
	}
	if len(w.failedSlots) != 0 {
		t.Fatalf("searched again at once: %v", w.failedSlots)
	}
}
