package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

type seededEp struct{ Item, Slot int64 }

type seededShow struct {
	Title, Version int64
	Eps            map[string]seededEp // "S01E02"
	FileID         int64               // the File on S01E01
}

// seedLibShow writes a show kept up to date with a main Version and four
// episodes: S01E01 aired and has a File, S01E02 and S02E01 aired without,
// S02E02 airs after now.
func seedLibShow(t *testing.T, s *database.Store, name string) seededShow {
	t.Helper()
	ctx := context.Background()
	lf := addShowFolder(t, s)
	out := seededShow{Eps: map[string]seededEp{}}
	order := OrderAired
	choice := SeasonsAll
	ti := mediafiles.Title{Kind: "show", Name: name, SortName: name, Aliases: []string{}, SourceIDs: map[string]string{"tmdb": name}, MetadataSource: "tmdb",
		EpisodeOrder: &order, KeepUpToDate: true, NewPartsPolicy: "all", ReleaseGate: GateReleased, MusicTypes: []string{}, WhyAdded: "by_hand",
		Artwork: []string{}, State: "active", GateDriver: "none", SeasonChoice: &choice, Status: "running"}
	eps := []struct {
		season, number int
		air            string
		file           bool
	}{{1, 1, "2026-10-01", true}, {1, 2, "2026-10-05", false}, {2, 1, "2026-10-10", false}, {2, 2, "2026-12-01", false}}
	err := s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := database.Insert(tx, "titles", &ti, domain.System); err != nil {
			return err
		}
		out.Title = ti.ID
		var pr int64
		if err := tx.QueryRow("SELECT id FROM pick_rules WHERE name = ?", mediafiles.GoodEverydayName).Scan(&pr); err != nil {
			return err
		}
		v := mediafiles.Version{TitleID: ti.ID, Label: "Main", PickRulesID: pr, LibraryFolderID: lf.ID, FolderName: name, KeepUpToDate: true, IsMain: true}
		if _, err := database.Insert(tx, "versions", &v, domain.System); err != nil {
			return err
		}
		out.Version = v.ID
		parts := map[int]int64{}
		for _, n := range []int{1, 2} {
			num := n
			p := mediafiles.Part{TitleID: ti.ID, Kind: "season", Number: &num, Name: "Season", KeepUpToDate: true, SourceIDs: map[string]string{}, Pressings: []mediafiles.Pressing{}}
			if _, err := database.Insert(tx, "parts", &p, domain.System); err != nil {
				return err
			}
			parts[n] = p.ID
		}
		for _, e := range eps {
			air, _ := time.Parse("2006-01-02", e.air)
			se, ep := e.season, e.number
			pid := parts[e.season]
			it := mediafiles.Item{TitleID: ti.ID, PartID: &pid, Kind: "episode", Numbers: mediafiles.Numbers{Season: &se, Episode: &ep}, AirDate: &air,
				KeepUpToDate: true, SourceIDs: map[string]string{}}
			if _, err := database.Insert(tx, "items", &it, domain.System); err != nil {
				return err
			}
			state := mediafiles.SlotWanted
			if e.air > "2026-10-13" {
				state = mediafiles.SlotNotOutYet
			}
			sl := mediafiles.Slot{ItemID: it.ID, VersionID: v.ID, State: state}
			if _, err := database.Insert(tx, "slots", &sl, domain.System); err != nil {
				return err
			}
			if e.file {
				grp := "GRP"
				f := mediafiles.File{SlotIDs: []int64{sl.ID}, VersionID: v.ID, RelativePath: "S01E01.mkv", Size: 1000, AddedAt: now, Languages: []string{}, Group: &grp,
					Origin: "imported", PlacedBy: "hardlink", Role: "main", State: "present"}
				if _, err := database.Insert(tx, "files", &f, domain.System); err != nil {
					return err
				}
				if _, err := tx.Exec("UPDATE slots SET file_id = ?, state = 'in_library' WHERE id = ?", f.ID, sl.ID); err != nil {
					return err
				}
				out.FileID = f.ID
			}
			out.Eps[fmt.Sprintf("S%02dE%02d", e.season, e.number)] = seededEp{Item: it.ID, Slot: sl.ID}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestStoreEpisodeLists(t *testing.T) {
	st := open(t)
	ctx := context.Background()
	sh := seedLibShow(t, st, "Lab Show")
	db := st.Read()
	e := func(k string) seededEp { return sh.Eps[k] }
	allItems := []int64{e("S01E01").Item, e("S01E02").Item, e("S02E01").Item, e("S02E02").Item}
	allSlots := []int64{e("S01E01").Slot, e("S01E02").Slot, e("S02E01").Slot, e("S02E02").Slot}

	// Missing: aired, no File, newest first; the future episode and the one with a File stay out.
	p, err := MissingEpisodes(ctx, db, true, "i.air_date", true, now, 1, 50)
	if err != nil || p.Total != 2 || !slices.Equal(p.IDs, []int64{e("S02E01").Item, e("S01E02").Item}) {
		t.Fatalf("missing desc %+v %v", p, err)
	}
	p, _ = MissingEpisodes(ctx, db, true, "i.air_date", false, now, 1, 50)
	if !slices.Equal(p.IDs, []int64{e("S01E02").Item, e("S02E01").Item}) {
		t.Fatalf("missing asc %+v", p)
	}
	// Paging: one per page; a page past the end is empty with the total kept.
	p, _ = MissingEpisodes(ctx, db, true, "i.air_date", true, now, 2, 1)
	if p.Total != 2 || !slices.Equal(p.IDs, []int64{e("S01E02").Item}) {
		t.Fatalf("page 2 %+v", p)
	}
	p, err = MissingEpisodes(ctx, db, true, "i.air_date", true, now, 99, 1)
	if err != nil || p.Total != 2 || len(p.IDs) != 0 {
		t.Fatalf("page past the end %+v %v", p, err)
	}
	if _, err := MissingEpisodes(ctx, db, true, "i.id; DROP TABLE items", true, now, 1, 50); !errors.Is(err, ErrMissingSort) {
		t.Fatalf("unlisted sort column: %v", err)
	}
	// Not kept: switching one episode off moves it to the other list.
	if err := st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE items SET keep_up_to_date = 0 WHERE id = ?", e("S01E02").Item)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if p, _ = MissingEpisodes(ctx, db, true, "i.air_date", true, now, 1, 50); p.Total != 1 {
		t.Fatalf("kept after one switched off %+v", p)
	}
	if p, _ = MissingEpisodes(ctx, db, false, "i.air_date", true, now, 1, 50); p.Total != 1 || p.IDs[0] != e("S01E02").Item {
		t.Fatalf("not kept %+v", p)
	}

	// Season counts.
	cs, err := SeasonCounts(ctx, db, []int64{sh.Title}, now)
	if err != nil || len(cs) != 2 {
		t.Fatalf("season counts %+v %v", cs, err)
	}
	by := map[int]SeasonCount{}
	for _, c := range cs {
		by[c.Season] = c
	}
	if c := by[1]; c.TitleID != sh.Title || c.Total != 2 || c.WithFile != 1 || c.Size != 1000 || c.ReleaseGroups != "GRP" {
		t.Fatalf("season 1 %+v", c)
	}
	if c := by[2]; c.Total != 2 || c.WithFile != 0 || c.Wanted != 1 || c.NextAiring == nil || c.PreviousAired == nil {
		t.Fatalf("season 2 %+v", c)
	}
	if cs, err := SeasonCounts(ctx, db, nil, now); err != nil || len(cs) != 0 {
		t.Fatalf("no titles %+v %v", cs, err)
	}

	// Slots of episodes, in id order; an empty list is an empty answer.
	slots, err := EpisodeSearchSlots(ctx, db, allItems)
	if err != nil || len(slots) != len(allItems) || !slices.IsSortedFunc(slots, func(a, b mediafiles.Slot) int { return int(a.ID - b.ID) }) {
		t.Fatalf("episode slots %v %v", slots, err)
	}
	if slots, err := EpisodeSearchSlots(ctx, db, nil); err != nil || len(slots) != 0 {
		t.Fatalf("empty ids %v %v", slots, err)
	}
	items, err := SlotItems(ctx, db, allSlots)
	if err != nil || len(items) != 4 {
		t.Fatalf("slot items %+v %v", items, err)
	}
	for _, si := range items {
		if si.TitleID != sh.Title || si.ItemKind != "episode" || si.Season == nil {
			t.Fatalf("slot item %+v", si)
		}
	}
	if items, err := SlotItems(ctx, db, nil); err != nil || len(items) != 0 {
		t.Fatalf("empty slots %+v %v", items, err)
	}

	// Main files: only the episode with a File.
	mf, err := MainFileOfItems(ctx, db, allItems)
	if err != nil || len(mf) != 1 || mf[e("S01E01").Item] != sh.FileID {
		t.Fatalf("main files %v %v", mf, err)
	}
	if mf, err := MainFileOfItems(ctx, db, nil); err != nil || len(mf) != 0 {
		t.Fatalf("empty items %v %v", mf, err)
	}

	// Episode rows: in season and episode order; KeptShows narrows.
	rows, err := EpisodeRows(ctx, db, EpisodeFilter{Where: "1 = 1"})
	if err != nil || len(rows) != 4 || *rows[0].Numbers.Episode != 1 || *rows[3].Numbers.Season != 2 || *rows[3].Numbers.Episode != 2 {
		t.Fatalf("episode rows %v %v", rows, err)
	}
	if rows, _ = EpisodeRows(ctx, db, EpisodeFilter{Where: "1 = 1", KeptShows: true}); len(rows) != 3 {
		t.Fatalf("kept rows %d, want 3 (one switched off)", len(rows))
	}
	if rows, _ = EpisodeRows(ctx, db, EpisodeFilter{Where: "id = ?", Args: []any{e("S02E01").Item}}); len(rows) != 1 {
		t.Fatalf("filtered rows %d", len(rows))
	}

	// A removed show vanishes from every list.
	if err := st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE titles SET state = 'removed' WHERE id = ?", sh.Title)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if slots, _ := EpisodeSearchSlots(ctx, db, allItems); len(slots) != 0 {
		t.Fatalf("removed show still has slots %v", slots)
	}
	if items, _ := SlotItems(ctx, db, allSlots); len(items) != 0 {
		t.Fatalf("removed show still has slot items %v", items)
	}
	if p, _ := MissingEpisodes(ctx, db, true, "i.air_date", true, now, 1, 50); p.Total != 0 {
		t.Fatalf("removed show still missing %+v", p)
	}
}

func TestStoreMovieLists(t *testing.T) {
	st := open(t)
	ctx := context.Background()
	addFolder(t, st)
	id, err := AddMovie(ctx, st, dune, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	db := st.Read()
	slots, err := MovieSearchSlots(ctx, db, []int64{id})
	if err != nil || len(slots) != 1 {
		t.Fatalf("movie slots %v %v", slots, err)
	}
	if slots, err := MovieSearchSlots(ctx, db, nil); err != nil || len(slots) != 0 {
		t.Fatalf("empty ids %v %v", slots, err)
	}
	if fs, err := MovieMainFiles(ctx, db, []int64{id}); err != nil || len(fs) != 0 {
		t.Fatalf("files before any File %v %v", fs, err)
	}
	var fid int64
	err = st.Write(ctx, func(tx *sql.Tx) error {
		f := mediafiles.File{SlotIDs: []int64{slots[0].ID}, VersionID: slots[0].VersionID, RelativePath: "m.mkv", Size: 5, AddedAt: now, Languages: []string{},
			Origin: "imported", PlacedBy: "hardlink", Role: "main", State: "present"}
		if _, err := database.Insert(tx, "files", &f, domain.System); err != nil {
			return err
		}
		fid = f.ID
		_, err := tx.Exec("UPDATE slots SET file_id = ? WHERE id = ?", f.ID, slots[0].ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if fs, err := MovieMainFiles(ctx, db, []int64{id}); err != nil || len(fs) != 1 || fs[0].ID != fid {
		t.Fatalf("movie files %v %v", fs, err)
	}
	if fs, err := MovieMainFiles(ctx, db, nil); err != nil || len(fs) != 0 {
		t.Fatalf("empty ids %v %v", fs, err)
	}
}
