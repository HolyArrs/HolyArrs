package mediafiles

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// seeded is one Title with one Slot, and the rows around it.
type seeded struct {
	Title Title
	Part  *Part
	Item  Item
	Ver   Version
	Slot  Slot
}

// seedSlot adds a Title of kind (movie or show) in titleState, with one Item
// and one Slot in slotState. A show's Item sits in a season Part.
func seedSlot(t *testing.T, s *database.Store, kind, name, titleState string, slotState SlotState) seeded {
	t.Helper()
	var out seeded
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		var folderID, rulesID int64
		if err := tx.QueryRow("SELECT id FROM library_folders ORDER BY id LIMIT 1").Scan(&folderID); err != nil {
			var err error
			folderID, err = database.Insert(tx, "library_folders", &LibraryFolder{Path: t.TempDir(), Kind: kind, Label: "f", Health: "ok", SameDiskAs: []int64{}}, database.System)
			if err != nil {
				return err
			}
		}
		if err := tx.QueryRow("SELECT id FROM pick_rules ORDER BY id LIMIT 1").Scan(&rulesID); err != nil {
			var err error
			rulesID, err = database.Insert(tx, "pick_rules", &PickRules{Name: "r", Kinds: []string{"movie", "show"}, Revision: 1, Body: PickRulesBody{}}, database.System)
			if err != nil {
				return err
			}
		}
		out.Title = Title{Kind: kind, Name: name, SortName: name, Aliases: []string{}, SourceIDs: map[string]string{}, MetadataSource: "local", NewPartsPolicy: "all", ReleaseGate: "released", GateDriver: "none", WhyAdded: "by_hand", Artwork: []string{}, State: titleState, Status: "released", MusicTypes: []string{}}
		if _, err := database.Insert(tx, "titles", &out.Title, database.System); err != nil {
			return err
		}
		itemKind := "movie"
		out.Item = Item{TitleID: out.Title.ID, Kind: itemKind, Numbers: Numbers{}, KeepUpToDate: true, SourceIDs: map[string]string{}}
		if kind == "show" {
			n := 1
			p := &Part{TitleID: out.Title.ID, Kind: "season", Number: &n, Name: "Season 1", KeepUpToDate: true, SourceIDs: map[string]string{}, Pressings: []Pressing{}}
			if _, err := database.Insert(tx, "parts", p, database.System); err != nil {
				return err
			}
			out.Part = p
			out.Item.Kind = "episode"
			out.Item.PartID = &p.ID
		}
		if _, err := database.Insert(tx, "items", &out.Item, database.System); err != nil {
			return err
		}
		out.Ver = Version{TitleID: out.Title.ID, Label: "HD", PickRulesID: rulesID, LibraryFolderID: folderID, FolderName: name, KeepUpToDate: true, IsMain: true, AudioLanguages: []string{}}
		if _, err := database.Insert(tx, "versions", &out.Ver, database.System); err != nil {
			return err
		}
		out.Slot = Slot{ItemID: out.Item.ID, VersionID: out.Ver.ID, State: slotState}
		_, err := database.Insert(tx, "slots", &out.Slot, database.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// seedRelease adds a Source and one Release, and returns the Release id.
func seedRelease(t *testing.T, s *database.Store, name string) int64 {
	t.Helper()
	var id int64
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		now := database.FormatTime(time.Now())
		res, err := tx.Exec("INSERT INTO sources (created_at, created_by, updated_at, updated_by, name, protocol, format, url) VALUES (?1, 't', ?1, 't', ?2, 'torrent', 'torznab', 'http://a')", now, name)
		if err != nil {
			return err
		}
		sid, _ := res.LastInsertId()
		res, err = tx.Exec("INSERT INTO releases (created_at, created_by, updated_at, updated_by, source_id, name, link_ref, protocol, found_by) VALUES (?1, 't', ?1, 't', ?2, ?3, 'x', 'torrent', 'feed')", now, sid, name)
		if err != nil {
			return err
		}
		id, _ = res.LastInsertId()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func setSlotUpdated(t *testing.T, s *database.Store, slotID int64, at time.Time) {
	t.Helper()
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE slots SET updated_at = ? WHERE id = ?", database.FormatTime(at), slotID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func slotIDs(slots []Slot) []int64 {
	out := []int64{}
	for _, sl := range slots {
		out = append(out, sl.ID)
	}
	return out
}

func TestHeldSlotsDueRoundTrip(t *testing.T) {
	s, _ := openTest(t, database.Options{})
	ctx := context.Background()
	a := seedSlot(t, s, "movie", "Held One", "active", SlotHeld)
	rel := seedRelease(t, s, "rel")
	now := time.Now().UTC()
	if err := s.Write(ctx, func(tx *sql.Tx) error { return SetSlotHeldTx(tx, a.Slot.ID, rel, now.Add(time.Hour)) }); err != nil {
		t.Fatal(err)
	}
	if due, err := HeldSlotDue(ctx, s.Read(), now); err != nil || due {
		t.Fatalf("due before the wait is over: %v %v", due, err)
	}
	if got, err := HeldSlotsDue(ctx, s.Read(), now); err != nil || len(got) != 0 {
		t.Fatalf("slots due early: %v %v", got, err)
	}
	later := now.Add(2 * time.Hour)
	if due, err := HeldSlotDue(ctx, s.Read(), later); err != nil || !due {
		t.Fatalf("not due after the wait: %v %v", due, err)
	}
	got, err := HeldSlotsDue(ctx, s.Read(), later)
	if err != nil || len(got) != 1 || got[0].ID != a.Slot.ID {
		t.Fatalf("slots due: %v %v", slotIDs(got), err)
	}
	held, err := HeldReleaseOf(ctx, s.Read(), a.Slot.ID)
	if err != nil || !held.Valid || held.Int64 != rel {
		t.Fatalf("held release: %v %v", held, err)
	}
	if err := s.Write(ctx, func(tx *sql.Tx) error { return ClearSlotHeldTx(tx, a.Slot.ID) }); err != nil {
		t.Fatal(err)
	}
	if held, _ := HeldReleaseOf(ctx, s.Read(), a.Slot.ID); held.Valid {
		t.Fatal("still holds a release after clearing")
	}
	if got, _ := HeldSlotsDue(ctx, s.Read(), later); len(got) != 0 {
		t.Fatalf("cleared slot still due: %v", slotIDs(got))
	}
}

func TestSlotsHeldByFiltersState(t *testing.T) {
	s, _ := openTest(t, database.Options{})
	ctx := context.Background()
	rel := seedRelease(t, s, "rel")
	_, _, _, withFile := addMovieCopy(t, s, "Has File", "av1", "ok", false)
	other := seedSlot(t, s, "movie", "Wanted One", "active", SlotWanted)
	err := s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE slots SET state = 'held', held_release_id = ? WHERE id = ?", rel, withFile.ID); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE slots SET held_release_id = ? WHERE id = ?", rel, other.Slot.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	err = s.Write(ctx, func(tx *sql.Tx) error {
		got, err := SlotsHeldBy(tx, rel)
		if err != nil {
			return err
		}
		if len(got) != 1 || got[0].ID != withFile.ID || !got[0].HasFile {
			t.Fatalf("held by: %+v", got)
		}
		none, err := SlotsHeldBy(tx, rel+99)
		if err != nil || len(none) != 0 {
			t.Fatalf("other release: %+v %v", none, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTitlesLookingAndKindWanted(t *testing.T) {
	s, _ := openTest(t, database.Options{})
	ctx := context.Background()
	movie := seedSlot(t, s, "movie", "Active Movie", "active", SlotWanted)
	seedSlot(t, s, "movie", "Removed Movie", "removed", SlotWanted)
	show := seedSlot(t, s, "show", "Active Show", "active", SlotInLibrary)
	states := [3]string{"wanted", "better_wanted", "held"}
	titles, err := TitlesLooking(ctx, s.Read(), states)
	if err != nil || len(titles) != 1 || titles[0].ID != movie.Title.ID {
		t.Fatalf("titles looking: %+v %v", titles, err)
	}
	slots, err := LookingSlotsOfTitle(ctx, s.Read(), movie.Title.ID, states)
	if err != nil || len(slots) != 1 || slots[0].ID != movie.Slot.ID {
		t.Fatalf("looking slots: %v %v", slotIDs(slots), err)
	}
	if slots, _ := LookingSlotsOfTitle(ctx, s.Read(), show.Title.ID, states); len(slots) != 0 {
		t.Fatalf("show slot that is in the library counted as looking: %v", slotIDs(slots))
	}
	if !KindWanted(ctx, s.Read(), "movie") {
		t.Fatal("movie kind should be wanted")
	}
	if KindWanted(ctx, s.Read(), "show") {
		t.Fatal("show kind should not be wanted")
	}
}

func TestSlotsSearchingBeforeAndRetryFacts(t *testing.T) {
	s, _ := openTest(t, database.Options{})
	ctx := context.Background()
	t0 := time.Now().UTC().Add(-5 * time.Hour)
	old := seedSlot(t, s, "movie", "Old Search", "active", SlotSearching)
	fresh := seedSlot(t, s, "movie", "Fresh Search", "active", SlotSearching)
	setSlotUpdated(t, s, old.Slot.ID, t0)
	setSlotUpdated(t, s, fresh.Slot.ID, t0.Add(2*time.Hour))
	got, err := SlotsSearchingBefore(ctx, s.Read(), t0.Add(time.Hour))
	if err != nil || len(got) != 1 || got[0].ID != old.Slot.ID {
		t.Fatalf("searching before: %v %v", slotIDs(got), err)
	}
	vid, part, dl, kind, err := SlotRetryFacts(ctx, s.Read(), old.Slot.ID)
	if err != nil || vid != old.Ver.ID || part.Valid || dl.Valid || kind != "movie" {
		t.Fatalf("retry facts: %v %v %v %q %v", vid, part, dl, kind, err)
	}
	show := seedSlot(t, s, "show", "Some Show", "active", SlotWanted)
	vid, part, _, kind, err = SlotRetryFacts(ctx, s.Read(), show.Slot.ID)
	if err != nil || vid != show.Ver.ID || !part.Valid || part.Int64 != show.Part.ID || kind != "show" {
		t.Fatalf("show retry facts: %v %v %q %v", vid, part, kind, err)
	}
	in, err := VersionSlotsOfPart(ctx, s.Read(), show.Ver.ID, show.Part.ID)
	if err != nil || len(in) != 1 || in[0].ID != show.Slot.ID {
		t.Fatalf("version slots of part: %v %v", slotIDs(in), err)
	}
	if in, _ := VersionSlotsOfPart(ctx, s.Read(), old.Ver.ID, show.Part.ID); len(in) != 0 {
		t.Fatalf("wrong version matched: %v", slotIDs(in))
	}
}

func needsDecisionEvents(t *testing.T, s *database.Store) []database.Event {
	t.Helper()
	es, _, err := s.Events(context.Background(), database.EventFilter{Kinds: []string{EventNeedsDecision}})
	if err != nil {
		t.Fatal(err)
	}
	return es
}

func TestMoveSlotNeedsDecisionEventOnce(t *testing.T) {
	s, _ := openTest(t, database.Options{})
	ctx := context.Background()
	a := seedSlot(t, s, "movie", "Stuck One", "active", SlotSearching)
	if err := MoveSlot(ctx, s, a.Slot.ID, SlotNeedsADecision, "why", database.System); err != nil {
		t.Fatal(err)
	}
	es := needsDecisionEvents(t, s)
	if len(es) != 1 {
		t.Fatalf("events after the move: %d", len(es))
	}
	e := es[0]
	if e.SlotID == nil || *e.SlotID != a.Slot.ID || e.TitleID == nil || *e.TitleID != a.Title.ID || e.Data["reason"] != "why" {
		t.Fatalf("event: %+v", e)
	}
	// A move to the same state is refused or ignored, and writes nothing more.
	_ = MoveSlot(ctx, s, a.Slot.ID, SlotNeedsADecision, "again", database.System)
	if n := len(needsDecisionEvents(t, s)); n != 1 {
		t.Fatalf("events after a same-state move: %d", n)
	}
	// A rolled-back move writes no event.
	b := seedSlot(t, s, "movie", "Rolled Back", "active", SlotSearching)
	boom := errors.New("boom")
	err := s.Write(ctx, func(tx *sql.Tx) error {
		if err := MoveSlotTx(tx, b.Slot.ID, SlotNeedsADecision, "no", database.System); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("write: %v", err)
	}
	if n := len(needsDecisionEvents(t, s)); n != 1 {
		t.Fatalf("events after a rollback: %d", n)
	}
	// A move to another state writes no needs-decision event.
	c := seedSlot(t, s, "movie", "Plain Move", "active", SlotWanted)
	if err := MoveSlot(ctx, s, c.Slot.ID, SlotSearching, "", database.System); err != nil {
		t.Fatal(err)
	}
	if n := len(needsDecisionEvents(t, s)); n != 1 {
		t.Fatalf("events after a plain move: %d", n)
	}
}

func TestSlotWriteHelpersKeepState(t *testing.T) {
	s, _ := openTest(t, database.Options{})
	ctx := context.Background()
	sd := seedSlot(t, s, "movie", "Writes", "active", SlotWanted)
	rel := seedRelease(t, s, "rel")
	t0 := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	err := s.Write(ctx, func(tx *sql.Tx) error {
		if err := SetSlotReasonTx(tx, sd.Slot.ID, "x", database.Actor{Kind: "system"}); err != nil {
			return err
		}
		if err := SetSlotChosenTx(tx, sd.Slot.ID, rel); err != nil {
			return err
		}
		return TouchItemSearchedTx(tx, sd.Item.ID, t0)
	})
	if err != nil {
		t.Fatal(err)
	}
	st, err := SlotStateOf(ctx, s.Read(), sd.Slot.ID)
	if err != nil || st != "wanted" {
		t.Fatalf("state %q %v", st, err)
	}
	var reason, by string
	var chosen int64
	if err := s.Read().QueryRow("SELECT last_reason, chosen_release_id, updated_by FROM slots WHERE id = ?", sd.Slot.ID).Scan(&reason, &chosen, &by); err != nil {
		t.Fatal(err)
	}
	var who database.Actor
	if reason != "x" || chosen != rel || json.Unmarshal([]byte(by), &who) != nil || who.Kind != "system" {
		t.Fatalf("reason %q chosen %d by %q", reason, chosen, by)
	}
	var searched string
	if err := s.Read().QueryRow("SELECT last_searched_at FROM items WHERE id = ?", sd.Item.ID).Scan(&searched); err != nil {
		t.Fatal(err)
	}
	if searched != database.FormatTime(t0) {
		t.Fatalf("searched %q", searched)
	}
}
