package sources

import (
	"context"
	"database/sql"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// TestTurnedDown_OnlyRefusalClearsWithTheList checks that a Release refused only because it was turned down is free again once the list is cleared, with no force needed.
func TestTurnedDown_OnlyRefusalClearsWithTheList(t *testing.T) {
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

// TestDangerousFiles_StayRefused checks that a clean-named Release whose files proved to be a program still needs force after the list is cleared.
// The same torrent under another name from another listing is refused too.
func TestDangerousFiles_StayRefused(t *testing.T) {
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
	runKind(t, f, sr, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slotID}, nil)
	if r := relByName(t, f, br1080); r.State == "chosen" || !hasCode(r, "hard.safety:dangerous_files") {
		t.Fatalf("same torrent under another name: %s %v", r.State, reasonCodes(r))
	}
	if st := slotState(t, f).State; st == mediafiles.SlotChosen {
		t.Fatal("the known program file was chosen again")
	}
}

// TestDangerousName_OtherHashStaysFree checks that a real torrent sharing a name with a known program file stays free when its info hash differs.
// Fakes copy scene names, so the hash is what counts.
func TestDangerousName_OtherHashStaysFree(t *testing.T) {
	const bad = "0123456789abcdef0123456789abcdef01234567"
	const good = "89abcdef0123456789abcdef0123456789abcdef"
	lf := &liveFeed{}
	f := seed(t, lf.url(t))
	a := addRelease(t, f, web1080, "turned_down")
	exec(t, f, `UPDATE releases SET info_hash = ?, link_ref = 'http://src/fake.torrent' WHERE id = ?`, bad, a)
	failedDownload(t, f, a, bad, "dangerous files: x.exe")
	sr := &Searcher{Store: f.s}
	if _, err := sr.RemoveTurnedDown(context.Background(), 0, domain.System); err != nil {
		t.Fatal(err)
	}
	lf.set(feedOf(item(web1080, time.Now(), 50, good)))
	if (dangerousRels{hashes: map[string]bool{bad: true}, names: map[string]bool{web1080: true}}).has(good, web1080) {
		t.Fatal("a listing with its own hash was matched by name")
	}
	if (dangerousRels{names: map[string]bool{web1080: true}}).has("", web1080) != true {
		t.Fatal("a hashless listing with the known name was not matched")
	}
	if got := listingHash("", "magnet:?xt=urn:btih:"+strings.ToUpper(bad)+"&dn=x"); got != bad {
		t.Fatalf("magnet hash %q", got)
	}
	runKind(t, f, sr, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slotID}, nil)
	var st, ver string
	if err := f.s.Read().QueryRow("SELECT state, COALESCE(verdict, '') FROM releases WHERE info_hash = ?", good).Scan(&st, &ver); err != nil {
		t.Fatalf("the real torrent was not stored: %v", err)
	}
	if strings.Contains(ver, "dangerous_files") || st == "refused" {
		t.Fatalf("the real torrent was refused by name: %s %s", st, ver)
	}
}

// TestHeldDangerous_NotSentAfterTheWait checks that a held Release whose torrent fails as dangerous during the wait is refused and its Slot looks again.
func TestHeldDangerous_NotSentAfterTheWait(t *testing.T) {
	const h = "0123456789abcdef0123456789abcdef01234567"
	f := seed(t, "http://127.0.0.1:1/api")
	held := addRelease(t, f, web1080, "held")
	exec(t, f, `UPDATE releases SET info_hash = ?, verdict = json('{"outcome":"held","reasons":[]}') WHERE id = ?`, h, held)
	exec(t, f, `UPDATE slots SET state = 'held', held_release_id = ?, next_try_at = ? WHERE id = ?`, held, database.FormatTime(time.Now().Add(-time.Minute)), f.slotID)
	other := addRelease(t, f, "Dune.Part.Two.2024.1080p.WEB-DL-OTHERNAME", "turned_down")
	failedDownload(t, f, other, h, "dangerous files: Dune.mkv.exe")
	var chosen []int64
	sr := &Searcher{Store: f.s, OnChosen: func(_ context.Context, _, rel int64) { chosen = append(chosen, rel) }}
	runKind(t, f, sr, jobs.KindGrabHeld, nil, nil)
	if len(chosen) != 0 {
		t.Fatalf("sent after the wait: %v", chosen)
	}
	r := relByName(t, f, web1080)
	if r.State != "refused" || !RefusedByRules(r) {
		t.Fatalf("held Release is %s", r.State)
	}
	if st := slotState(t, f).State; st != mediafiles.SlotWanted {
		t.Fatalf("slot %s", st)
	}
}

// TestListingHash_Base32 checks that a magnet link may carry its hash in base32.
func TestListingHash_Base32(t *testing.T) {
	const h = "0123456789abcdef0123456789abcdef01234567"
	b, _ := hex.DecodeString(h)
	if got := listingHash("", "magnet:?xt=urn:btih:"+base32.StdEncoding.EncodeToString(b)+"&dn=x"); got != h {
		t.Fatalf("got %q", got)
	}
	if got := listingHash("", "magnet:?xt=urn:btih:"+strings.ToUpper(h)); got != h {
		t.Fatalf("hex at the end: %q", got)
	}
}

func failedDownload(t *testing.T, f fixture, id int64, hash, why string) {
	t.Helper()
	if err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		d := &domain.Downloader{Name: "q", Adapter: "qbittorrent", Protocol: "torrent", URL: "http://q", Category: "c", PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok"}
		if _, err := database.Insert(tx, "downloaders", d, domain.System); err != nil {
			return err
		}
		dl := &domain.Download{DownloaderID: d.ID, ReleaseID: &id, SlotIDs: []int64{f.slotID}, ExternalID: hash, State: "failed", StateReason: &why,
			FilesSeen: []domain.SeenFile{}, StartedBy: domain.System}
		_, err := database.Insert(tx, "downloads", dl, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
