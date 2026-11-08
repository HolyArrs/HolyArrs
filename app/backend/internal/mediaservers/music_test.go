package mediaservers

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// 4.15.5: a music change is scoped to the album folder: an album import
// names the folders of the tracks it placed, a rename its new folder, a
// delete the folders it emptied; anything else is the artist folder.
func TestFoldersOfMusicScopedToAlbum(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	lib := filepath.Join(t.TempDir(), "music")
	var artist int64
	var slots []int64
	err := w.s.Write(ctx, func(tx *sql.Tx) error {
		var pr int64
		_ = tx.QueryRow("SELECT id FROM pick_rules LIMIT 1").Scan(&pr)
		lf := &mediafiles.LibraryFolder{Path: lib, Kind: "music", Label: "Music", Health: "ok", SameDiskAs: []int64{}}
		if _, err := database.Insert(tx, "library_folders", lf, domain.System); err != nil {
			return err
		}
		ti := &mediafiles.Title{Kind: "artist", GateDriver: "none", Name: "Lab Lanterns", SortName: "lab lanterns", MetadataSource: "musicbrainz", NewPartsPolicy: "all",
			ReleaseGate: "released", WhyAdded: "by_hand", Status: "running", State: "active", SourceIDs: map[string]string{}, Aliases: []string{}, MusicTypes: []string{}, Artwork: []string{}}
		if _, err := database.Insert(tx, "titles", ti, domain.System); err != nil {
			return err
		}
		artist = ti.ID
		v := &mediafiles.Version{TitleID: ti.ID, Label: "Main", PickRulesID: pr, LibraryFolderID: lf.ID, FolderName: "Lab Lanterns", IsMain: true}
		if _, err := database.Insert(tx, "versions", v, domain.System); err != nil {
			return err
		}
		for i, rel := range []string{"Lab Lanterns/Small Hours (2025)/01 - A.flac", "Lab Lanterns/Small Hours (2025)/CD2/01 - B.flac"} {
			n := i + 1
			it := &mediafiles.Item{TitleID: ti.ID, Kind: "track", Numbers: mediafiles.Numbers{Track: &n}, SourceIDs: map[string]string{}}
			if _, err := database.Insert(tx, "items", it, domain.System); err != nil {
				return err
			}
			sl := &mediafiles.Slot{ItemID: it.ID, VersionID: v.ID, State: mediafiles.SlotInLibrary}
			if _, err := database.Insert(tx, "slots", sl, domain.System); err != nil {
				return err
			}
			f := &mediafiles.File{SlotIDs: []int64{sl.ID}, VersionID: v.ID, RelativePath: rel, Languages: []string{}, MediaInfo: map[string]any{}, Origin: "imported",
				PlacedBy: "hardlink", Role: "main", State: "present"}
			if _, err := database.Insert(tx, "files", f, domain.System); err != nil {
				return err
			}
			if _, err := tx.Exec("UPDATE slots SET file_id = ? WHERE id = ?", f.ID, sl.ID); err != nil {
				return err
			}
			slots = append(slots, sl.ID)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	l := &Linker{Store: w.s, Vault: w.v}
	album := filepath.Join(lib, "Lab Lanterns", "Small Hours (2025)")
	for _, c := range []struct {
		data map[string]any
		want string
	}{
		{map[string]any{"placed_slot_ids": []any{float64(slots[0]), float64(slots[1])}}, fmt.Sprint([]string{album})},
		{map[string]any{"to": "Lab Lanterns/Long Night (2026)/01 - C.flac"}, fmt.Sprint([]string{filepath.Join(lib, "Lab Lanterns", "Long Night (2026)")})},
		{map[string]any{"folders": []any{"Lab Lanterns/Small Hours (2025)/", "Lab Lanterns/Small Hours (2025)/CD2/"}}, fmt.Sprint([]string{album})},
		{map[string]any{"folders": []any{"../../etc/"}}, fmt.Sprint([]string{filepath.Join(lib, "Lab Lanterns")})},
		{map[string]any{"to": `Lab Lanterns/x\..\..\Private/01.flac`}, fmt.Sprint([]string{filepath.Join(lib, "Lab Lanterns")})},
		{map[string]any{}, fmt.Sprint([]string{filepath.Join(lib, "Lab Lanterns")})},
	} {
		got := l.FoldersOf(ctx, domain.Event{Kind: "import_done", TitleID: &artist, Data: c.data})
		if fmt.Sprint(got) != c.want {
			t.Errorf("%v: %v, want %v", c.data, got, c.want)
		}
	}
	// A movie keeps its own folder.
	if got := l.FoldersOf(ctx, domain.Event{Kind: "import_done", TitleID: &w.title}); len(got) != 1 || got[0] != w.folder() {
		t.Fatalf("movie %v", got)
	}
}
