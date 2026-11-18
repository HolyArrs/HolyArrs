package mediafiles

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// catalogWorld is one movie title in one library folder with three
// versions: v1 (folder "B"), v2 (main, folder "A") and v3 (no folder).
// v1 holds slot s1 with no file; v2 holds slot s2 with a protected file.
type catalogWorld struct {
	s              *database.Store
	root           string
	title          int64
	v1, v2, v3     int64
	s1, s2, fileID int64
}

func newCatalogWorld(t *testing.T) catalogWorld {
	t.Helper()
	s, _ := openTest(t, database.Options{})
	w := catalogWorld{s: s, root: t.TempDir()}
	ctx := context.Background()
	err := s.Write(ctx, func(tx *sql.Tx) error {
		lf := LibraryFolder{Path: w.root, Kind: "movie", Label: "Movies", Health: "ok", SameDiskAs: []int64{}}
		lfID, err := database.Insert(tx, "library_folders", &lf, database.System)
		if err != nil {
			return err
		}
		var rules int64
		if err := tx.QueryRow("SELECT MIN(id) FROM pick_rules").Scan(&rules); err != nil {
			return err
		}
		ti := Title{Kind: "movie", Name: "Lab Movie", SortName: "lab movie", Aliases: []string{}, SourceIDs: map[string]string{}, MetadataSource: "tmdb", WhyAdded: "by_hand", State: "active"}
		ti.NewPartsPolicy, ti.ReleaseGate, ti.GateDriver, ti.Status = "all", "released", "none", "announced"
		if w.title, err = database.Insert(tx, "titles", &ti, database.System); err != nil {
			return err
		}
		ver := func(folder string, main bool) (int64, error) {
			v := Version{TitleID: w.title, Label: folder, PickRulesID: rules, LibraryFolderID: lfID, FolderName: folder, IsMain: main, AudioLanguages: []string{}}
			return database.Insert(tx, "versions", &v, database.System)
		}
		if w.v1, err = ver("B", false); err != nil {
			return err
		}
		if w.v2, err = ver("A", true); err != nil {
			return err
		}
		if w.v3, err = ver("", false); err != nil {
			return err
		}
		it := Item{TitleID: w.title, Kind: "movie", SourceIDs: map[string]string{}}
		itemID, err := database.Insert(tx, "items", &it, database.System)
		if err != nil {
			return err
		}
		s1 := Slot{ItemID: itemID, VersionID: w.v1, State: SlotWanted}
		if w.s1, err = database.Insert(tx, "slots", &s1, database.System); err != nil {
			return err
		}
		s2 := Slot{ItemID: itemID, VersionID: w.v2, State: SlotWanted}
		if w.s2, err = database.Insert(tx, "slots", &s2, database.System); err != nil {
			return err
		}
		f := File{Protected: true, SlotIDs: []int64{w.s2}, VersionID: w.v2, RelativePath: "A/Lab Movie.mkv", Size: 10, AddedAt: time.Now().UTC(),
			Languages: []string{}, MediaInfo: map[string]any{}, Origin: "imported", PlacedBy: "hardlink", Role: "main", State: "present"}
		if w.fileID, err = database.Insert(tx, "files", &f, database.System); err != nil {
			return err
		}
		_, err = tx.Exec("UPDATE slots SET file_id = ?, state = ? WHERE id = ?", w.fileID, SlotInLibrary, w.s2)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestTitleFoldersMainFirstSkipsEmpty(t *testing.T) {
	w := newCatalogWorld(t)
	ctx := context.Background()
	got, err := TitleFolders(ctx, w.s.Read(), w.title)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{filepath.Join(w.root, "A"), filepath.Join(w.root, "B")}; !reflect.DeepEqual(got, want) {
		t.Fatalf("folders %v, want %v", got, want)
	}
	kind, root, folder, err := MainFolder(ctx, w.s.Read(), w.title)
	if err != nil || kind != "movie" || root != w.root || folder != "A" {
		t.Fatalf("main folder %q %q %q %v", kind, root, folder, err)
	}
	rel, err := SlotPath(ctx, w.s.Read(), w.s2)
	if err != nil || rel != "A/Lab Movie.mkv" {
		t.Fatalf("slot path %q %v", rel, err)
	}
	if _, err := SlotPath(ctx, w.s.Read(), w.s1); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("slot without file: %v", err)
	}
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE versions SET is_main = 0 WHERE title_id = ?", w.title)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := MainFolder(ctx, w.s.Read(), w.title); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("no main version: %v", err)
	}
}

func TestSlotCopyProtectedNoFileAndProtected(t *testing.T) {
	w := newCatalogWorld(t)
	ctx := context.Background()
	if p, err := SlotCopyProtected(ctx, w.s.Read(), w.s1); err != nil || p {
		t.Fatalf("slot without file: %v %v", p, err)
	}
	if p, err := SlotCopyProtected(ctx, w.s.Read(), w.s2); err != nil || !p {
		t.Fatalf("protected slot: %v %v", p, err)
	}
	err := w.s.Write(ctx, func(tx *sql.Tx) error {
		ss, ts, p, err := SlotChoiceFacts(tx, w.s2)
		if err != nil {
			return err
		}
		if ss != string(SlotInLibrary) || ts != "active" || !p {
			t.Errorf("facts s2: %q %q %v", ss, ts, p)
		}
		ss, _, p, err = SlotChoiceFacts(tx, w.s1)
		if err != nil {
			return err
		}
		if ss != string(SlotWanted) || p {
			t.Errorf("facts s1: %q %v", ss, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTitleInFolderAndMarkSeasonsDiffer(t *testing.T) {
	w := newCatalogWorld(t)
	ctx := context.Background()
	if !TitleInFolder(ctx, w.s.Read(), w.title, w.root, "A") || TitleInFolder(ctx, w.s.Read(), w.title, w.root, "Z") {
		t.Fatal("TitleInFolder")
	}
	if err := w.s.Write(ctx, func(tx *sql.Tx) error { return MarkSeasonsDifferTx(ctx, tx, w.title) }); err != nil {
		t.Fatal(err)
	}
	if got := SeasonsDiffer(ctx, w.s.Read(), w.title); got != 1 {
		t.Fatalf("differ = %d", got)
	}
}

// FolderSourceTags lists every copy of a title with the tags of its Library
// folder when that folder asks only tagged Sources, and no tags for a copy in
// another folder.
func TestFolderSourceTags(t *testing.T) {
	w := newCatalogWorld(t)
	ctx := context.Background()
	set := func(q string, args ...any) {
		t.Helper()
		if err := w.s.Write(ctx, func(tx *sql.Tx) error { _, err := tx.Exec(q, args...); return err }); err != nil {
			t.Fatal(err)
		}
	}
	set(`UPDATE library_folders SET new_title_tags = '["anime","subs"]'`)
	got, err := FolderSourceTags(ctx, w.s.Read(), w.title)
	if err != nil || len(got) != 0 {
		t.Fatalf("no folder has the switch on, and copies were still listed: %v %v", got, err)
	}
	set(`UPDATE library_folders SET only_tagged_sources = 1`)
	// The third copy moves to a plain folder.
	set(`INSERT INTO library_folders (path, kind, label, health, same_disk_as, created_at, updated_at, created_by, updated_by)
		SELECT path || '2', kind, 'Plain', health, same_disk_as, created_at, updated_at, created_by, updated_by FROM library_folders`)
	set(`UPDATE versions SET library_folder_id = (SELECT max(id) FROM library_folders) WHERE id = ?`, w.v3)
	for _, id := range []int64{0, w.title} {
		got, err := FolderSourceTags(ctx, w.s.Read(), id)
		if err != nil || !reflect.DeepEqual(CleanTags(got[w.title][w.v1]), []string{"anime", "subs"}) || !reflect.DeepEqual(CleanTags(got[w.title][w.v2]), []string{"anime", "subs"}) {
			t.Fatalf("switch on (%d): %v %v", id, got, err)
		}
		if tags, ok := got[w.title][w.v3]; !ok || len(tags) != 0 {
			t.Fatalf("the copy in the plain folder (%d): %v %v", id, tags, ok)
		}
	}
	if got, _ := FolderSourceTags(ctx, w.s.Read(), w.title+1); len(got) != 0 {
		t.Fatalf("another title: %v", got)
	}
}
