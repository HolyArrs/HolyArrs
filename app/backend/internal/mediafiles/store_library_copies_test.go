package mediafiles

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

func addMovieCopy(t *testing.T, s *database.Store, name, codec, inspection string, protected bool) (Title, Version, File, Slot) {
	t.Helper()
	var title Title
	var version Version
	var file File
	var slot Slot
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		var folderID, rulesID int64
		if err := tx.QueryRow("SELECT id FROM library_folders ORDER BY id LIMIT 1").Scan(&folderID); err != nil {
			folder := &LibraryFolder{Path: t.TempDir(), Kind: "movie", Label: name, Health: "ok", SameDiskAs: []int64{}}
			var err error
			folderID, err = database.Insert(tx, "library_folders", folder, database.System)
			if err != nil {
				return err
			}
		}
		if err := tx.QueryRow("SELECT id FROM pick_rules ORDER BY id LIMIT 1").Scan(&rulesID); err != nil {
			rules := &PickRules{Name: name, Kinds: []string{"movie"}, Revision: 1, Body: PickRulesBody{}}
			var err error
			rulesID, err = database.Insert(tx, "pick_rules", rules, database.System)
			if err != nil {
				return err
			}
		}
		title = Title{Kind: "movie", Name: name, SortName: name, Aliases: []string{}, SourceIDs: map[string]string{}, MetadataSource: "local", NewPartsPolicy: "all", ReleaseGate: "released", GateDriver: "none", WhyAdded: "by_hand", Artwork: []string{}, State: "active", Status: "released", MusicTypes: []string{}}
		if _, err := database.Insert(tx, "titles", &title, database.System); err != nil {
			return err
		}
		item := &Item{TitleID: title.ID, Kind: "movie", Numbers: Numbers{}, KeepUpToDate: true, SourceIDs: map[string]string{}}
		if _, err := database.Insert(tx, "items", item, database.System); err != nil {
			return err
		}
		version = Version{TitleID: title.ID, Label: "4K", PickRulesID: rulesID, LibraryFolderID: folderID, FolderName: name, KeepUpToDate: true, IsMain: true, Edition: "Director's cut", AudioLanguages: []string{"en"}}
		if _, err := database.Insert(tx, "versions", &version, database.System); err != nil {
			return err
		}
		file = File{VersionID: version.ID, Protected: protected, SlotIDs: []int64{}, RelativePath: name + ".mkv", Size: 42, AddedAt: time.Now().UTC(), Quality: Quality{Codec: "av1"}, Languages: []string{}, MediaInfo: map[string]any{"inspection_status": inspection, "container": "matroska", "video_codec": codec, "audio_languages": []string{"ja", "en"}, "audio_tracks": 2, "audio_unknown": false}, Origin: "imported", PlacedBy: "move", Role: "main", State: "present"}
		if _, err := database.Insert(tx, "files", &file, database.System); err != nil {
			return err
		}
		slot = Slot{ItemID: item.ID, VersionID: version.ID, State: SlotInLibrary, FileID: &file.ID}
		if _, err := database.Insert(tx, "slots", &slot, database.System); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE files SET slot_ids = json_array(?) WHERE id = ?", slot.ID, file.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return title, version, file, slot
}

func TestLibraryCopiesUsesOnlySuccessfulInspectedCodec(t *testing.T) {
	s, _ := openTest(t, database.Options{})
	addMovieCopy(t, s, "Arrival", "hevc", "complete", false)
	addMovieCopy(t, s, "Error Film", "h264", "error", false)
	addMovieCopy(t, s, "Parsed Only", "", "complete", false)

	page, err := LibraryCopies(context.Background(), s, LibraryCopyFilter{Sort: "codec", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 3 || len(page.Copies) != 3 {
		t.Fatalf("page %+v", page)
	}
	if page.Copies[0].Codec != "hevc" || page.Copies[0].TitleName != "Arrival" {
		t.Fatalf("first %+v", page.Copies[0])
	}
	if page.Copies[1].Codec != "unknown" || page.Copies[2].Codec != "unknown" {
		t.Fatalf("unknown codecs %+v", page.Copies)
	}
	if got := page.Codecs["unknown"]; got != 2 {
		t.Fatalf("unknown count %d", got)
	}
	if got := page.Copies[0].AudioLanguages; len(got) != 2 || got[0] != "ja" || got[1] != "en" {
		t.Fatalf("audio languages %#v", got)
	}
}

func TestLibraryCopiesFiltersSearchesAndPagesDeterministically(t *testing.T) {
	s, _ := openTest(t, database.Options{})
	_, v1, f1, _ := addMovieCopy(t, s, "Alien", "h264", "partial", false)
	addMovieCopy(t, s, "Aliens", "hevc", "complete", false)
	addMovieCopy(t, s, "Dune", "hevc", "complete", false)

	page, err := LibraryCopies(context.Background(), s, LibraryCopyFilter{Query: "Ali", Codec: "h264", Sort: "title", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Copies) != 1 || page.Copies[0].FileID != f1.ID {
		t.Fatalf("page %+v", page)
	}
	got, err := SelectedLibraryCopies(context.Background(), s, []LibraryCopySelection{{FileID: f1.ID, VersionID: v1.ID}})
	if err != nil || len(got) != 1 || got[0].FileID != f1.ID {
		t.Fatalf("selected %+v err %v", got, err)
	}
	if _, err := SelectedLibraryCopies(context.Background(), s, []LibraryCopySelection{{FileID: f1.ID, VersionID: v1.ID + 99}}); err == nil {
		t.Fatal("mismatched Version accepted")
	}

	if _, err := LibraryCopies(context.Background(), s, LibraryCopyFilter{Codec: "h265"}); err == nil {
		t.Fatal("invalid codec accepted")
	}
	if _, err := LibraryCopies(context.Background(), s, LibraryCopyFilter{Sort: "size"}); err == nil {
		t.Fatal("invalid sort accepted")
	}
}

func TestLibraryCopySlotsAreExactMovieVersion(t *testing.T) {
	s, _ := openTest(t, database.Options{})
	_, v, f, slot := addMovieCopy(t, s, "Heat", "h264", "complete", false)
	ids, err := LibraryCopySlots(context.Background(), s, LibraryCopySelection{FileID: f.ID, VersionID: v.ID})
	if err != nil || len(ids) != 1 || ids[0] != slot.ID {
		t.Fatalf("slots %#v err %v", ids, err)
	}
}

func TestTitleCopiesGivesEachMovieItsMainFile(t *testing.T) {
	s, _ := openTest(t, database.Options{})
	arrival, av, af, _ := addMovieCopy(t, s, "Arrival", "hevc", "complete", true)
	broken, _, _, _ := addMovieCopy(t, s, "Broken", "h264", "error", false)

	got, err := TitleCopies(context.Background(), s.Read())
	if err != nil {
		t.Fatal(err)
	}
	a, ok := got[arrival.ID]
	if !ok || a.FileID != af.ID || a.VersionID != av.ID || a.Codec != "hevc" || a.Container != "matroska" || !a.Protected {
		t.Fatalf("arrival %+v (found %v)", a, ok)
	}
	if len(a.AudioLanguages) != 2 || a.AudioLanguages[0] != "ja" {
		t.Fatalf("audio %#v", a.AudioLanguages)
	}
	if b := got[broken.ID]; b.Codec != "unknown" || b.AudioLanguages == nil {
		t.Fatalf("broken %+v", b)
	}
}
