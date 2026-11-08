package sources

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Pins of the queries in sources that read slots, items, titles, versions and
// files (mediafiles tables). They must give the same answers after the reads
// move behind mediafiles functions.

// bulkMovies adds n kept-up-to-date wanted movie slots (one title, item,
// version each). It returns the slot ids in insert order.
func bulkMovies(t *testing.T, f fixture, n int) []int64 {
	t.Helper()
	var ids []int64
	err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		var pr, lf int64
		if err := tx.QueryRow("SELECT id FROM pick_rules LIMIT 1").Scan(&pr); err != nil {
			return err
		}
		if err := tx.QueryRow("SELECT id FROM library_folders LIMIT 1").Scan(&lf); err != nil {
			return err
		}
		for i := 0; i < n; i++ {
			name := fmt.Sprintf("Bulk Movie %04d", i)
			ti := &mediafiles.Title{Kind: "movie", GateDriver: "none", Name: name, SortName: name, Year: ptr(2000 + i%20),
				Aliases: []string{}, SourceIDs: map[string]string{}, MetadataSource: "local", KeepUpToDate: true, NewPartsPolicy: "all",
				ReleaseGate: "released", MusicTypes: []string{}, WhyAdded: "by_hand", Artwork: []string{}, Status: "released", State: "active"}
			if _, err := database.Insert(tx, "titles", ti, domain.System); err != nil {
				return err
			}
			it := &mediafiles.Item{TitleID: ti.ID, Kind: "movie", KeepUpToDate: true}
			if _, err := database.Insert(tx, "items", it, domain.System); err != nil {
				return err
			}
			v := &mediafiles.Version{TitleID: ti.ID, Label: "HD", PickRulesID: pr, LibraryFolderID: lf, FolderName: name, KeepUpToDate: true, IsMain: true}
			if _, err := database.Insert(tx, "versions", v, domain.System); err != nil {
				return err
			}
			sl := &mediafiles.Slot{ItemID: it.ID, VersionID: v.ID, State: mediafiles.SlotWanted}
			if _, err := database.Insert(tx, "slots", sl, domain.System); err != nil {
				return err
			}
			ids = append(ids, sl.ID)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

func jobCount(t *testing.T, f fixture, kind string, subjectID int64) int {
	t.Helper()
	var n int
	if err := f.s.Read().QueryRow("SELECT count(*) FROM jobs WHERE kind = ? AND json_extract(subject, '$.id') = ?", kind, subjectID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// The scheduled missing search over a library of over 1,000 slots: only
// kept, active, wanted slots count, never-searched first, then by slot id.
func TestPinMissingSearchLargeLibrary(t *testing.T) {
	f := seed(t, "http://127.0.0.1:1/api")
	ids := bulkMovies(t, f, 1200)
	// The first 600 bulk slots were searched before, so they go last.
	old, _ := json.Marshal(ids[:600])
	exec(t, f, `UPDATE items SET last_searched_at = '2020-01-01T00:00:00Z' WHERE id IN (SELECT item_id FROM slots WHERE id IN (SELECT value FROM json_each(?)))`, string(old))
	// Decoys that must not count: item not kept up to date, title not active.
	exec(t, f, "UPDATE items SET keep_up_to_date = 0 WHERE id = (SELECT item_id FROM slots WHERE id = ?)", ids[700])
	exec(t, f, "UPDATE titles SET state = 'removed' WHERE id = (SELECT i.title_id FROM slots s JOIN items i ON i.id = s.item_id WHERE s.id = ?)", ids[701])
	setSetting(t, f, SettingMissingCount, "300")
	j := runKind(t, f, &Searcher{Store: f.s}, jobs.KindSearchMissing, nil, nil)
	if j.State != "done" {
		t.Fatalf("%s %+v", j.State, j.Error)
	}
	if n, _ := j.Result["slots"].(float64); n != 300 {
		t.Fatalf("slots %+v", j.Result)
	}
	// Never searched, by id: the fixture slot, then bulk slots from index 600 on.
	want := []int64{f.slotID}
	for _, id := range ids[600:] {
		if len(want) == 300 {
			break
		}
		if id != ids[700] && id != ids[701] {
			want = append(want, id)
		}
	}
	rows, err := f.s.Read().Query(`SELECT json_extract(subject, '$.id') FROM jobs WHERE kind = ?`, jobs.KindSearchSlot)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		got = append(got, id)
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("queued %d slots, differs from expected %d", len(got), len(want))
	}
}

// A show season with several wanted episodes is searched as one season job;
// a season with one aired episode gets an episode search; not-out-yet
// episodes are left out.
func TestPinMissingSearchGroupsSeasons(t *testing.T) {
	f := seedShow(t, "http://127.0.0.1:1/api", nil)
	j := runKind(t, f.fixture, &Searcher{Store: f.s}, jobs.KindSearchMissing, nil, nil)
	if j.State != "done" {
		t.Fatalf("%s %+v", j.State, j.Error)
	}
	if s, _ := j.Result["seasons"].(float64); s != 1 {
		t.Fatalf("seasons %+v", j.Result)
	}
	if n := jobCount(t, f.fixture, jobs.KindSearchSeason, f.parts[1]); n != 1 {
		t.Fatalf("season 1 jobs %d", n)
	}
	if n := jobCount(t, f.fixture, jobs.KindSearchSlot, f.slots["S02E01"]); n != 1 {
		t.Fatalf("S02E01 jobs %d", n)
	}
	if n := jobCount(t, f.fixture, jobs.KindSearchSlot, f.slots["S02E02"]); n != 0 {
		t.Fatalf("future episode searched %d", n)
	}
	if n := jobCount(t, f.fixture, jobs.KindSearchSlot, f.slotID); n != 1 {
		t.Fatalf("movie jobs %d", n)
	}
}

// The copy guard read: slot join item join title, LEFT JOIN files. A slot with
// no file (NULL partner) is not protected; the right slot is found among 1,200.
func TestPinChoiceStateAndProtectedCopy(t *testing.T) {
	f := seed(t, "http://127.0.0.1:1/api")
	ids := bulkMovies(t, f, 1200)
	ctx := context.Background()
	var versionID int64
	if err := f.s.Read().QueryRow("SELECT version_id FROM slots WHERE id = ?", ids[900]).Scan(&versionID); err != nil {
		t.Fatal(err)
	}
	protectSlotCopy(t, f, ids[900], versionID)
	check := func(slot int64) (mediafiles.SlotState, error) {
		var st mediafiles.SlotState
		err := f.s.Write(ctx, func(tx *sql.Tx) error {
			var e error
			st, e = choiceStateTx(tx, slot)
			return e
		})
		return st, err
	}
	if st, err := check(ids[10]); err != nil || st != mediafiles.SlotWanted {
		t.Fatalf("null file slot: %v %v", st, err)
	}
	if st, err := check(ids[900]); err == nil || st != mediafiles.SlotBetterWanted {
		t.Fatalf("protected slot: %v %v", st, err)
	}
	if slotHasProtectedCopy(ctx, f.s.Read(), ids[10]) || !slotHasProtectedCopy(ctx, f.s.Read(), ids[900]) {
		t.Fatal("protected copy read changed")
	}
	if slotHasProtectedCopy(ctx, f.s.Read(), 999999) {
		t.Fatal("missing slot reads as protected")
	}
	exec(t, f, "UPDATE titles SET state = 'removed' WHERE id = (SELECT i.title_id FROM slots s JOIN items i ON i.id = s.item_id WHERE s.id = ?)", ids[20])
	if _, err := check(ids[20]); err == nil {
		t.Fatal("removed title chosen")
	}
}

// The Held list: page by hold_until then id, naming the title and version
// through slot joins. A release with no matched slot has no names.
func TestPinListedReleasesTitleNames(t *testing.T) {
	f := seed(t, "http://127.0.0.1:1/api")
	ids := bulkMovies(t, f, 1100)
	err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		for i, sid := range ids {
			r := &domain.Release{SourceID: f.srcID, Name: fmt.Sprintf("Rel.%04d.1080p", i), LinkRef: "x", Protocol: "torrent", Flags: []string{},
				Parsed: map[string]any{}, MatchedSlotIDs: []int64{sid}, Verdict: map[string]any{}, FoundBy: "hand_search", State: "held"}
			if _, err := database.Insert(tx, "releases", r, domain.System); err != nil {
				return err
			}
		}
		orphan := &domain.Release{SourceID: f.srcID, Name: "Orphan.1080p", LinkRef: "x", Protocol: "torrent", Flags: []string{},
			Parsed: map[string]any{}, MatchedSlotIDs: []int64{}, Verdict: map[string]any{}, FoundBy: "hand_search", State: "held"}
		_, err := database.Insert(tx, "releases", orphan, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	sr := &Searcher{Store: f.s}
	list, total, err := sr.List(context.Background(), "held", 50, 0)
	if err != nil || total != 1101 || len(list) != 50 {
		t.Fatalf("list %d of %d: %v", len(list), total, err)
	}
	for _, l := range list {
		if l.SourceName != "Test" || l.SlotID != l.MatchedSlotIDs[0] || l.TitleName == "" || l.Version != "HD" || l.TitleID == 0 {
			t.Fatalf("names for %s: %+v", l.Name, l)
		}
	}
	// The orphan is the last by id (hold_until is NULL for all).
	last, _, err := sr.List(context.Background(), "held", 50, 1100)
	if err != nil || len(last) != 1 {
		t.Fatalf("last page %d %v", len(last), err)
	}
	if l := last[0]; l.Name != "Orphan.1080p" || l.TitleName != "" || l.Version != "" || l.TitleID != 0 || l.SlotID != 0 {
		t.Fatalf("orphan got names: %+v", l)
	}
}

// packSlots reads the title kind of the first matched slot.
func TestPinPackSlotsReadsTitleKind(t *testing.T) {
	f := seedShow(t, "http://127.0.0.1:1/api", nil)
	sr := &Searcher{Store: f.s}
	ctx := context.Background()
	movie := domain.Release{Name: "Dune.Part.Two.2024.1080p.WEB-GRP", MatchedSlotIDs: []int64{f.slotID}}
	if out, _, kind := sr.packSlots(ctx, movie); kind != "movie" || out != nil {
		t.Fatalf("movie: %v %q", out, kind)
	}
	show := domain.Release{Name: "Copper.Valley.S01.1080p.WEB-GRP", Protocol: "torrent", MatchedSlotIDs: []int64{f.slots["S01E01"], f.slots["S01E02"], f.slots["S01E03"]}}
	out, _, kind := sr.packSlots(ctx, show)
	if kind != "show" || len(out) != 3 {
		t.Fatalf("show: %v %q", out, kind)
	}
	show.Verdict = map[string]any{"turned_down_slot_ids": []any{float64(f.slots["S01E02"])}}
	out, _, _ = sr.packSlots(ctx, show)
	if len(out) != 2 {
		t.Fatalf("turned down kept: %v", out)
	}
}

// QueueAfterFailure reads the slot's version, part, download and title kind.
// A slot with no download gets its own slot search; an unknown slot errors.
func TestPinQueueAfterFailureByKind(t *testing.T) {
	f := seedShow(t, "http://127.0.0.1:1/api", nil)
	q := jobs.New(f.s, jobs.Options{Workers: 1, BaseBackoff: time.Hour, PollInterval: time.Hour})
	sr := &Searcher{Store: f.s, Queue: q}
	ctx := context.Background()
	for _, id := range []int64{f.slotID, f.slots["S01E01"]} {
		if err := sr.QueueAfterFailure(ctx, id, domain.System); err != nil {
			t.Fatal(err)
		}
		if n := jobCount(t, f.fixture, jobs.KindSearchSlot, id); n != 1 {
			t.Fatalf("slot %d jobs %d", id, n)
		}
	}
	if err := sr.QueueAfterFailure(ctx, 999999, domain.System); err == nil {
		t.Fatal("unknown slot accepted")
	}
}
