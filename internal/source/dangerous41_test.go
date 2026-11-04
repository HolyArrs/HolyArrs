package source

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// failedDownload records a failed Download of release id with reason why.
func failedDownload(t *testing.T, f fixture, id int64, hash, why string) {
	t.Helper()
	if err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		d := &domain.Downloader{Name: "q", Adapter: "qbittorrent", Protocol: "torrent", URL: "http://q", Category: "c", PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok"}
		if _, err := store.Insert(tx, "downloaders", d, domain.System); err != nil {
			return err
		}
		dl := &domain.Download{DownloaderID: d.ID, ReleaseID: &id, SlotIDs: []int64{f.slotID}, ExternalID: hash, State: "failed", StateReason: &why,
			FilesSeen: []domain.SeenFile{}, StartedBy: domain.System}
		_, err := store.Insert(tx, "downloads", dl, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

// #41: a Release refused only because it was turned down before is free
// again once the Turned down list is cleared: no force needed.
func TestTurnedDownOnlyRefusalClearsWithTheList(t *testing.T) {
	f := seed(t, "http://127.0.0.1:1/api")
	ctx := context.Background()
	b := addRelease(t, f, web1080, "turned_down")
	exec(t, f, `UPDATE releases SET verdict = json('{"outcome":"refused","passed":false,"reasons":[{"step":"hard","rule":"safety","effect":"pass"},{"step":"hard","rule":"turned_down","effect":"fail","detail":"this Release was turned down for this Slot before"}]}') WHERE id = ?`, b)
	failedDownload(t, f, b, "", "stalled")
	sr := &Searcher{Store: f.s}
	if _, err := sr.RemoveTurnedDown(ctx, 0, domain.System); err != nil {
		t.Fatal(err)
	}
	r := relByName(t, f, web1080)
	if r.State != "passed_over" || RefusedByRules(r) {
		t.Fatalf("after clearing: %s, refused by rules %v", r.State, RefusedByRules(r))
	}
	if _, err := sr.Choose(ctx, b, f.slotID, false, domain.System); err != nil {
		t.Fatalf("needs force for a turn-down that was cleared: %v", err)
	}
}

// #41: a clean-named Release whose files turned out to be a program
// (fail_dangerous) still needs force after the list is cleared, and the
// same torrent under another name from another listing is refused too.
func TestDangerousFilesStayRefused(t *testing.T) {
	const h = "0123456789abcdef0123456789abcdef01234567"
	lf := &liveFeed{}
	f := seed(t, lf.url(t))
	ctx := context.Background()
	a := addRelease(t, f, web1080, "turned_down")
	exec(t, f, `UPDATE releases SET info_hash = ?, verdict = json('{"outcome":"chosen","passed":true,"reasons":[]}') WHERE id = ?`, h, a)
	failedDownload(t, f, a, h, "dangerous files: Dune.mkv.exe")
	sr := &Searcher{Store: f.s}
	if _, err := sr.RemoveTurnedDown(ctx, 0, domain.System); err != nil {
		t.Fatal(err)
	}
	if r := relByName(t, f, web1080); r.State != "refused" {
		t.Fatalf("after clearing: %s", r.State)
	}
	var nf *NeedsForceError
	if _, err := sr.Choose(ctx, a, f.slotID, false, domain.System); !errors.As(err, &nf) {
		t.Fatalf("picked without force: %v", err)
	}
	lf.set(feedOf(item(br1080, time.Now(), 50, h)))
	runKind(t, f, sr, KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slotID}, nil)
	if r := relByName(t, f, br1080); r.State == "chosen" || !hasCode(r, "hard.safety:dangerous_files") {
		t.Fatalf("same torrent under another name: %s %v", r.State, reasonCodes(r))
	}
	if st := slotState(t, f).State; st == domain.SlotChosen {
		t.Fatal("the known program file was chosen again")
	}
}
