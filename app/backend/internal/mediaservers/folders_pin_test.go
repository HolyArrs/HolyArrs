package mediaservers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Pins of the folder reads the link code does over versions, library_folders,
// titles, slots and files (mediafiles tables).

func pinFolderRoot(t *testing.T, w *world) int64 {
	t.Helper()
	var lf int64
	if err := w.s.Read().QueryRow("SELECT id FROM library_folders LIMIT 1").Scan(&lf); err != nil {
		t.Fatal(err)
	}
	return lf
}

func pinTitle(kind, name string) *mediafiles.Title {
	return &mediafiles.Title{Kind: kind, GateDriver: "none", Name: name, SortName: strings.ToLower(name), MetadataSource: "local", NewPartsPolicy: "all",
		ReleaseGate: "released", WhyAdded: "by_hand", Status: "released", State: "active", SourceIDs: map[string]string{}, Aliases: []string{},
		MusicTypes: []string{}, Artwork: []string{}}
}

// Folders: one folder per version, main version first, blank names left out,
// a title with no version gives nothing; the right title among 1,200.
func TestPinFoldersOfTitle(t *testing.T) {
	w := newWorld(t)
	lf := pinFolderRoot(t, w)
	var lastTitle int64
	err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		var pr int64
		_ = tx.QueryRow("SELECT id FROM pick_rules LIMIT 1").Scan(&pr)
		for i := 0; i < 1200; i++ {
			ti := pinTitle("movie", fmt.Sprintf("M%04d", i))
			if _, err := database.Insert(tx, "titles", ti, domain.System); err != nil {
				return err
			}
			lastTitle = ti.ID
			if _, err := database.Insert(tx, "versions", &mediafiles.Version{TitleID: ti.ID, Label: "HD", PickRulesID: pr, LibraryFolderID: lf, FolderName: fmt.Sprintf("M%04d", i), IsMain: i%2 == 0}, domain.System); err != nil {
				return err
			}
		}
		// The last title also has a second (non-main) version and a blank one.
		for _, v := range []mediafiles.Version{{Label: "4K", FolderName: "Second"}, {Label: "Blank", FolderName: ""}} {
			v.TitleID, v.PickRulesID, v.LibraryFolderID = lastTitle, pr, lf
			if _, err := database.Insert(tx, "versions", &v, domain.System); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	l := &Linker{Store: w.s}
	got := l.Folders(context.Background(), lastTitle)
	// i = 1199 is odd, so its first version is not main: no main version, so
	// the order is by version id.
	want := []string{filepath.Join(w.lib, "M1199"), filepath.Join(w.lib, "Second")}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("folders %v want %v", got, want)
	}
	if got := l.Folders(context.Background(), w.title); len(got) != 1 || got[0] != w.folder() {
		t.Fatalf("arrival folders %v", got)
	}
	if got := l.Folders(context.Background(), 987654); len(got) != 0 {
		t.Fatalf("unknown title %v", got)
	}
}

// FoldersOf for an artist names album folders from the event, or from the
// files of the slots it names; with nothing usable it falls back to the
// title's folders. A slot with no file (NULL partner) adds nothing.
func TestPinFoldersOfArtist(t *testing.T) {
	w := newWorld(t)
	lf := pinFolderRoot(t, w)
	var artist, slotFile, slotNoFile int64
	err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		var pr int64
		_ = tx.QueryRow("SELECT id FROM pick_rules LIMIT 1").Scan(&pr)
		ti := pinTitle("artist", "Band")
		if _, err := database.Insert(tx, "titles", ti, domain.System); err != nil {
			return err
		}
		artist = ti.ID
		v := &mediafiles.Version{TitleID: ti.ID, Label: "Lossless", PickRulesID: pr, LibraryFolderID: lf, FolderName: "Band", IsMain: true}
		if _, err := database.Insert(tx, "versions", v, domain.System); err != nil {
			return err
		}
		var slots []*mediafiles.Slot
		for i := 0; i < 2; i++ {
			it := &mediafiles.Item{TitleID: ti.ID, Kind: "track", KeepUpToDate: true}
			if _, err := database.Insert(tx, "items", it, domain.System); err != nil {
				return err
			}
			sl := &mediafiles.Slot{ItemID: it.ID, VersionID: v.ID, State: mediafiles.SlotWanted}
			if _, err := database.Insert(tx, "slots", sl, domain.System); err != nil {
				return err
			}
			slots = append(slots, sl)
		}
		fl := &mediafiles.File{SlotIDs: []int64{slots[0].ID}, VersionID: v.ID, RelativePath: "Band/Album One/01 Song.flac", Size: 1, AddedAt: time.Now(),
			Languages: []string{}, MediaInfo: map[string]any{}, Origin: "imported", PlacedBy: "move", Role: "main", State: "present"}
		if _, err := database.Insert(tx, "files", fl, domain.System); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE slots SET file_id = ? WHERE id = ?", fl.ID, slots[0].ID); err != nil {
			return err
		}
		slotFile, slotNoFile = slots[0].ID, slots[1].ID
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	l := &Linker{Store: w.s}
	ctx := context.Background()
	ev := func(data map[string]any) domain.Event {
		return domain.Event{Kind: "import_done", TitleID: &artist, Data: data}
	}
	album := filepath.Join(w.lib, "Band", "Album One")
	if got := l.FoldersOf(ctx, ev(map[string]any{"placed_slot_ids": []any{float64(slotFile)}})); len(got) != 1 || got[0] != album {
		t.Fatalf("by slot file: %v", got)
	}
	// A slot without a file gives no album, so the title's folder is used.
	if got := l.FoldersOf(ctx, ev(map[string]any{"placed_slot_ids": []any{float64(slotNoFile)}})); len(got) != 1 || got[0] != filepath.Join(w.lib, "Band") {
		t.Fatalf("null file: %v", got)
	}
	bad := []string{"Band/Album Two/a.flac", "Band/Album Two/b.flac", "Other/Elsewhere/x.flac", "Band/../x/y.flac", "Band\\Bad/x.flac"}
	if got := l.FoldersOf(ctx, ev(map[string]any{"folders": bad})); len(got) != 1 || got[0] != filepath.Join(w.lib, "Band", "Album Two") {
		t.Fatalf("event folders: %v", got)
	}
	// A loose file in the artist folder (two parts) names the artist folder.
	if got := l.FoldersOf(ctx, ev(map[string]any{"folders": []string{"Band/Loose.flac"}})); len(got) != 1 || got[0] != filepath.Join(w.lib, "Band") {
		t.Fatalf("loose file: %v", got)
	}
	if got := l.FoldersOf(ctx, ev(map[string]any{"to": "Band/Album Three/x.flac"})); len(got) != 1 || got[0] != filepath.Join(w.lib, "Band", "Album Three") {
		t.Fatalf("rename target: %v", got)
	}
	// A movie title always takes the plain folders.
	mv := domain.Event{Kind: "import_done", TitleID: &w.title}
	if got := l.FoldersOf(ctx, mv); len(got) != 1 || got[0] != w.folder() {
		t.Fatalf("movie: %v", got)
	}
}

// The JSON of the stored media server record is frozen with its field names.
func TestPinMediaServerRecordGolden(t *testing.T) {
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	ref := int64(7)
	ms := domain.MediaServer{Name: "Jellyfin", Kind: "jellyfin", URL: "http://jf:8096", TokenSecretID: &ref, Enabled: true, OnImport: true,
		OnRename: true, OnDelete: false, PathFrom: "/data", PathTo: "/media", AllowLocal: true, LastOKAt: &at, LastError: "slow"}
	ms.ID = 2
	ms.CreatedAt, ms.UpdatedAt = at, at
	got, err := json.MarshalIndent(ms, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", "pin_media_server_record.golden.json")
	if os.Getenv("PIN_UPDATE") == "1" {
		_ = os.MkdirAll("testdata", 0o755)
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ReplaceAll(string(want), "\r\n", "\n") != string(got) {
		t.Fatalf("record changed:\n%s", got)
	}
}
