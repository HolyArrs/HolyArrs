package sources

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// plainFolder adds a second Library folder, labelled Plain, and returns its id.
func plainFolder(t *testing.T, f fixture) int64 {
	t.Helper()
	exec(t, f, `INSERT INTO library_folders (path, kind, label, health, same_disk_as, created_at, updated_at, created_by, updated_by)
		SELECT path || '2', kind, 'Plain', health, same_disk_as, created_at, updated_at, created_by, updated_by FROM library_folders`)
	var id int64
	if err := f.s.Read().QueryRow("SELECT max(id) FROM library_folders").Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// copySlots gives the title a second copy in the Plain folder, with a Slot for each of the given ones; it returns the new Slots in the same order.
func copySlots(t *testing.T, f fixture, slots ...int64) []int64 {
	t.Helper()
	ctx := context.Background()
	folder := plainFolder(t, f)
	var first mediafiles.Slot
	var ver mediafiles.Version
	if err := database.Get(ctx, f.s.Read(), "slots", slots[0], &first); err != nil {
		t.Fatal(err)
	}
	if err := database.Get(ctx, f.s.Read(), "versions", first.VersionID, &ver); err != nil {
		t.Fatal(err)
	}
	var out []int64
	if err := f.s.Write(ctx, func(tx *sql.Tx) error {
		ver.ID, ver.Label, ver.IsMain, ver.FolderName, ver.LibraryFolderID = 0, "Second", false, ver.FolderName+" 2", folder
		if _, err := database.Insert(tx, "versions", &ver, domain.System); err != nil {
			return err
		}
		for _, id := range slots {
			var sl mediafiles.Slot
			if err := database.Get(ctx, tx, "slots", id, &sl); err != nil {
				return err
			}
			sl.ID, sl.VersionID = 0, ver.ID
			if _, err := database.Insert(tx, "slots", &sl, domain.System); err != nil {
				return err
			}
			out = append(out, sl.ID)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

// secondCopy gives the fixture's movie a second copy in the Plain folder and returns its Slot.
func secondCopy(t *testing.T, f fixture) int64 {
	t.Helper()
	return copySlots(t, f, f.slotID)[0]
}

func slotOf(t *testing.T, f fixture, id int64) mediafiles.Slot {
	t.Helper()
	var sl mediafiles.Slot
	if err := database.Get(context.Background(), f.s.Read(), "slots", id, &sl); err != nil {
		t.Fatal(err)
	}
	return sl
}

// A search by hand that cannot ask the folder's tagged Source asks nobody and
// tells the person why, as a finished search and not a failed one.
func TestHandle_FolderKeptApartSaysWhyByHand(t *testing.T) {
	general := &liveFeed{}
	general.set(duneFeed)
	f := seed(t, general.url(t))
	anime := &liveFeed{}
	anime.set(duneFeed)
	addTaggedSource(t, f, "Anime", anime.url(t), []string{"anime"}, func(src *domain.Source) { src.UseFor.HandSearch = false })
	exec(t, f, `UPDATE library_folders SET new_title_tags = '["anime"]', only_tagged_sources = 1`)
	j := runKind(t, f, &Searcher{Store: f.s}, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slotID}, map[string]any{"by_hand": true})
	want := "This folder only takes Sources tagged anime. None can be searched right now (Anime has search turned off), so nothing was asked."
	sl := slotOf(t, f, f.slotID)
	if j.State != "done" || feedHits(general) != 0 || feedHits(anime) != 0 || sl.LastReason == nil || *sl.LastReason != want {
		t.Fatalf("job %s, general asked %d, anime asked %d, reason %v", j.State, feedHits(general), feedHits(anime), sl.LastReason)
	}
	evs, _, err := f.s.Events(context.Background(), database.EventFilter{Limit: 5, Kinds: []string{EventSearchDone, EventSearchFailed}})
	if err != nil || len(evs) != 1 || evs[0].Kind != EventSearchDone || evs[0].Data["sources_note"] != want {
		t.Fatalf("events %+v %v", evs, err)
	}
}

// A show with one copy in a folder whose tag no Source has and another copy
// in a plain folder: only the first copy's feed pick says the tags were
// passed over, whichever copy comes first.
func TestFeedShow_NoteIsForTheCopy(t *testing.T) {
	for _, strict := range []string{"label != 'Plain'", "label = 'Plain'"} {
		ts := &tvServer{answer: func(url.Values) []string { return []string{"Copper.Valley.S01E01.1080p.WEB-DL.H.264-QUILL"} }}
		f := seedShow(t, ts.start(t), nil)
		first := f.slots["S01E01"]
		second := copySlots(t, f.fixture, first)[0]
		exec(t, f.fixture, `UPDATE library_folders SET new_title_tags = '["anmie"]', only_tagged_sources = 1 WHERE `+strict)
		tagged, plain := first, second
		if strings.HasPrefix(strict, "label =") {
			tagged, plain = second, first
		}
		if j := runKind(t, f.fixture, &Searcher{Store: f.s}, jobs.KindFeedCheck, nil, nil); j.State != "done" {
			t.Fatalf("feed %s %+v", j.State, j.Error)
		}
		evs, _, err := f.s.Events(context.Background(), database.EventFilter{Limit: 20, Kinds: []string{EventReleaseChosen, EventReleaseHeld}})
		if err != nil {
			t.Fatal(err)
		}
		notes := map[int64]string{}
		seen := map[int64]bool{}
		for _, e := range evs {
			if e.SlotID != nil {
				notes[*e.SlotID], _ = e.Data["sources_note"].(string)
				seen[*e.SlotID] = true
			}
		}
		if !seen[tagged] || !seen[plain] || notes[tagged] != tagsPassedOver || notes[plain] != "" {
			t.Fatalf("strict folder %s: picks %v, the tagged copy says %q, the plain copy says %q", strict, seen, notes[tagged], notes[plain])
		}
	}
}

// A title carrying as many tags as it may, in a folder with one more tag,
// still gets the feed Release of the Source that has the folder's tag.
func TestFeed_FolderTagKeptBesideAFullTitle(t *testing.T) {
	lf := &liveFeed{}
	lf.set(feedOf(item(web1080, time.Now().Add(-2*time.Hour), 50, "")))
	f := seed(t, lf.url(t))
	feedOn(t, f)
	var tags []string
	for i := 0; i < 20; i++ {
		tags = append(tags, fmt.Sprintf(`"a%02d"`, i))
	}
	exec(t, f, `UPDATE titles SET tags = ?`, "["+strings.Join(tags, ",")+"]")
	exec(t, f, `UPDATE sources SET tags = '["zanime"]'`)
	exec(t, f, `UPDATE library_folders SET new_title_tags = '["zanime"]', only_tagged_sources = 1`)
	runKind(t, f, &Searcher{Store: f.s}, jobs.KindFeedCheck, nil, nil)
	if rs, _ := database.Select[domain.Release](context.Background(), f.s.Read(), "releases", "name = ?", web1080); len(rs) != 1 {
		t.Fatalf("the tagged Source's Release reached the title %d times", len(rs))
	}
}

// An episode held from an untagged Source before its folder asked only tagged
// Sources is not picked up again by a later feed check.
func TestFeedShow_HeldFromAnUntaggedSourceIsLeftOut(t *testing.T) {
	quietTV := &tvServer{answer: func(url.Values) []string { return nil }}
	f := seedShow(t, quietTV.start(t), nil)
	animeTV := &tvServer{answer: func(url.Values) []string { return []string{"Copper.Valley.S01E02.1080p.WEB-DL.H.264-QUILL"} }}
	addTaggedSource(t, f.fixture, "Anime", animeTV.start(t), []string{"anime"}, nil)
	ctx := context.Background()
	held := f.slots["S01E01"]
	if err := f.s.Write(ctx, func(tx *sql.Tx) error {
		rel := &domain.Release{SourceID: f.srcID, Name: "Copper.Valley.S01E01.1080p.WEB-DL.H.264-QUILL", LinkRef: "x", Protocol: "torrent", Size: 4294967296, Seeders: ptr(50),
			Flags: []string{}, Parsed: map[string]any{}, MatchedSlotIDs: []int64{held}, FoundBy: "feed", State: "held", Verdict: map[string]any{}}
		if _, err := database.Insert(tx, "releases", rel, domain.System); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE slots SET state = 'held', held_release_id = ?, next_try_at = ? WHERE id = ?", rel.ID, database.FormatTime(time.Now().Add(time.Hour)), held)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	exec(t, f.fixture, `UPDATE library_folders SET new_title_tags = '["anime"]', only_tagged_sources = 1`)
	if j := runKind(t, f.fixture, &Searcher{Store: f.s}, jobs.KindFeedCheck, nil, nil); j.State != "done" {
		t.Fatalf("feed %s %+v", j.State, j.Error)
	}
	if st := f.state(t, "S01E02"); st != mediafiles.SlotChosen {
		t.Fatalf("the tagged Source's episode: %s", st)
	}
	if st := f.state(t, "S01E01"); st == mediafiles.SlotChosen {
		t.Fatal("the Release held from the untagged Source was chosen by the feed check")
	}
}

// Two copies hold one Release of an untagged Source; only the copy whose
// folder now asks only tagged Sources lets it go, the other one takes it.
func TestGrabHeld_OnlyTheCopyKeptApartLetsGo(t *testing.T) {
	lf := &liveFeed{}
	lf.set(feedOf())
	f := seed(t, lf.url(t))
	quiet := &liveFeed{}
	quiet.set(feedOf())
	addTaggedSource(t, f, "Anime", quiet.url(t), []string{"anime"}, nil)
	second := secondCopy(t, f)
	ctx := context.Background()
	if err := f.s.Write(ctx, func(tx *sql.Tx) error {
		rel := &domain.Release{SourceID: f.srcID, Name: web1080, LinkRef: "x", Protocol: "torrent", Flags: []string{}, Parsed: map[string]any{},
			MatchedSlotIDs: []int64{f.slotID, second}, FoundBy: "feed", State: "held", Verdict: map[string]any{}}
		if _, err := database.Insert(tx, "releases", rel, domain.System); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE slots SET state = 'held', held_release_id = ?, next_try_at = ?", rel.ID, database.FormatTime(time.Now().Add(-time.Minute)))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	exec(t, f, `UPDATE library_folders SET new_title_tags = '["anime"]', only_tagged_sources = 1 WHERE label != 'Plain'`)
	n, err := (&Searcher{Store: f.s}).grabHeld(ctx, domain.System, nil, time.Now().UTC())
	a, b := slotOf(t, f, f.slotID), slotOf(t, f, second)
	if err != nil || n != 1 || a.State != mediafiles.SlotWanted || b.State != mediafiles.SlotChosen {
		t.Fatalf("chosen %d %v: the copy kept apart is %s, the plain copy is %s", n, err, a.State, b.State)
	}
}
