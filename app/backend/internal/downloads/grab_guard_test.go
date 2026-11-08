package downloads

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestQbitRefusesIdsThatAreNotInfoHashes(t *testing.T) {
	f, u := startFake(t)
	f.set(hash, map[string]any{"name": "other", "category": "holyarrs", "state": "downloading"})
	q := NewQbittorrent(u, nil)
	ctx := context.Background()
	for _, id := range []string{"all", "a|b", hash + "|" + hash, "", "xyz"} {
		if _, err := q.Get(ctx, id); err == nil {
			t.Fatalf("Get(%q) answered", id)
		}
		if err := q.Remove(ctx, id, true); err == nil {
			t.Fatalf("Remove(%q) answered", id)
		}
	}
	if len(f.selected) != 0 || len(f.deleted) != 0 {
		t.Fatalf("hostile ids reached qBittorrent: %v %v", f.selected, f.deleted)
	}
	if id, err := q.Add(ctx, AddRequest{URL: "http://x/a.torrent", InfoHash: "all"}); err != nil || id != "" {
		t.Fatalf("Add gave %q %v for a bad hash", id, err)
	}
}

func TestGrabWithAHostileHash(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	w.fake.set(hash, map[string]any{"name": "someone else's", "category": "holyarrs", "state": "downloading"})
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE releases SET info_hash = 'all' WHERE id = ?", w.relID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Grab(ctx, w.slotID, w.relID, domain.System); err == nil {
		t.Fatal("grab with a hostile hash succeeded")
	}
	for _, s := range w.fake.selected {
		if s == "all" {
			t.Fatalf("selector all sent: %v", w.fake.selected)
		}
	}
	if w.countDownloads(t) != 0 || len(w.fake.deleted) != 0 {
		t.Fatalf("downloads %d deleted %v", w.countDownloads(t), w.fake.deleted)
	}
}

func TestGrabNeverOverwritesAnActiveDownload(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	var otherID int64
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		dl := &domain.Download{DownloaderID: 1, SlotIDs: []int64{999}, ExternalID: hash, State: "downloading",
			FilesSeen: []domain.SeenFile{}, SavePath: "/other", StartedBy: domain.System}
		_, err := database.Insert(tx, "downloads", dl, domain.System)
		otherID = dl.ID
		return err
	}); err != nil {
		t.Fatal(err)
	}
	w.fake.set(hash, map[string]any{"name": "x", "category": "holyarrs", "state": "downloading"})
	if _, err := m.Grab(ctx, w.slotID, w.relID, domain.System); err == nil {
		t.Fatal("grab took over an active Download")
	}
	d := w.download(t, otherID)
	if d.State != "downloading" || d.SavePath != "/other" || len(d.SlotIDs) != 1 || d.SlotIDs[0] != 999 || d.ReleaseID != nil {
		t.Fatalf("active row overwritten: %+v", d)
	}
	if w.slot(t).State != mediafiles.SlotChosen {
		t.Fatalf("slot moved: %s", w.slot(t).State)
	}
}

func TestStaleSyncNeverUndoesMarkFailed(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	id := grabbed(t, w, m)
	stale := w.download(t, id)
	if err := m.MarkFailed(ctx, id, domain.System, "luis"); err != nil {
		t.Fatal(err)
	}
	dr := w.downloader(t, stale.DownloaderID)
	if m.apply(ctx, dr, stale, Job{ID: hash, State: StateDownloading, Progress: 0.7, Speed: 10}, true, m.settings(ctx)) {
		t.Fatal("stale snapshot still counted as active")
	}
	if d := w.download(t, id); d.State != "failed" || d.Progress == 0.7 {
		t.Fatalf("stale sync wrote over the failed row: %s %v", d.State, d.Progress)
	}
}

func TestChangeRefusedWhileActive(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	grabbed(t, w, m)
	d := w.downloader(t, 1)
	cat := "other"
	if _, err := Change(ctx, w.s, nil, d.ID, Input{Name: d.Name, URL: d.URL, Category: &cat}, domain.System); !errors.Is(err, ErrBusy) {
		t.Fatalf("category change: %v", err)
	}
	if _, err := Change(ctx, w.s, nil, d.ID, Input{Name: d.Name, URL: "http://elsewhere:8080"}, domain.System); !errors.Is(err, ErrBusy) {
		t.Fatalf("host change: %v", err)
	}
	same := d.Category
	if _, err := Change(ctx, w.s, nil, d.ID, Input{Name: "Renamed", URL: d.URL, Category: &same}, domain.System); err != nil {
		t.Fatalf("rename refused: %v", err)
	}
}

func TestSabnzbdFailedJobNotAdoptedAgain(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	f := &fakeSab{history: "history.json"}
	u := startSab(t, f)
	m.FromVault = func(context.Context, int64) (string, error) { return sabKey, nil }
	var ref int64
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.Exec(`INSERT INTO secrets (kind, value, created_at, created_by, updated_at, updated_by) VALUES ('downloader_key', 'sealed', '', '{}', '', '{}')`)
		if err != nil {
			return err
		}
		ref, err = res.LastInsertId()
		return err
	}); err != nil {
		t.Fatal(err)
	}
	did := w.addDownloader(t, &domain.Downloader{Name: "Lab SABnzbd", Adapter: "sabnzbd", Protocol: "usenet", URL: u, Category: "movies", Priority: 1, AllowLocal: true, SecretRef: &ref})
	var relID int64
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		r := &domain.Release{SourceID: 1, Name: "Lab.Usenet.Movie.2024.1080p.WEB.H264-HOLYARRS", LinkRef: "http://fake-indexer/good.nzb", Protocol: "usenet",
			Size: 1238952, Flags: []string{}, Parsed: map[string]any{}, MatchedSlotIDs: []int64{w.slotID}, Verdict: map[string]any{}, FoundBy: "auto_search", State: "chosen"}
		if _, err := database.Insert(tx, "releases", r, domain.System); err != nil {
			return err
		}
		relID = r.ID
		if _, err := tx.Exec("UPDATE slots SET chosen_release_id = ? WHERE id = ?", r.ID, w.slotID); err != nil {
			return err
		}
		old := &domain.Download{DownloaderID: did, ReleaseID: &relID, SlotIDs: []int64{w.slotID}, ExternalID: sabGood, State: "failed",
			FilesSeen: []domain.SeenFile{}, StartedBy: domain.System}
		_, err := database.Insert(tx, "downloads", old, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Grab(ctx, w.slotID, relID, domain.System); err != nil {
		t.Fatal(err)
	}
	if len(f.added) != 1 {
		t.Fatalf("the failed job was adopted again instead of a fresh add: %v", f.calls)
	}
}
