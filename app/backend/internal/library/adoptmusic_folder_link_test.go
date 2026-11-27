package library

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// albumFolderWorld puts an artist in the Library with one album, "5:55", of
// three tracks, no file linked. Nothing on disk names the album, so a scan
// can never place the folder: only a person can.
func albumFolderWorld(t *testing.T) (*Service, *database.Store, mediafiles.LibraryFolder, int64, []int64) {
	t.Helper()
	svc, s, _, lf := musicService(t)
	var partID int64
	var slots []int64
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		var pr int64
		_ = tx.QueryRow("SELECT id FROM pick_rules ORDER BY id LIMIT 1").Scan(&pr)
		ti := &mediafiles.Title{Kind: "artist", GateDriver: "none", Name: "Charlotte Gainsbourg", SortName: "gainsbourg", Aliases: []string{},
			SourceIDs: map[string]string{"musicbrainz": "f0000000-0000-4000-8000-000000000001"}, MetadataSource: "musicbrainz", NewPartsPolicy: "all",
			ReleaseGate: "released", MusicTypes: []string{}, WhyAdded: "by_hand", Artwork: []string{}, Status: "running", State: "active", KeepUpToDate: true}
		if _, err := database.Insert(tx, "titles", ti, domain.System); err != nil {
			return err
		}
		v := &mediafiles.Version{TitleID: ti.ID, Label: "Main", PickRulesID: pr, LibraryFolderID: lf.ID, FolderName: "Charlotte Gainsbourg", IsMain: true, KeepUpToDate: true}
		if _, err := database.Insert(tx, "versions", v, domain.System); err != nil {
			return err
		}
		rd := time.Date(2006, 3, 1, 0, 0, 0, 0, time.UTC)
		p := &mediafiles.Part{TitleID: ti.ID, Kind: "album", Name: "5:55", ReleaseDate: &rd, KeepUpToDate: true,
			SourceIDs: map[string]string{"musicbrainz": "f1000000-0000-4000-8000-000000000001"}, Pressings: []mediafiles.Pressing{}, SecondaryTypes: []string{}, Included: true}
		if _, err := database.Insert(tx, "parts", p, domain.System); err != nil {
			return err
		}
		partID = p.ID
		for i, tn := range []string{"AF607105", "5:55", "The Operation"} {
			disc, num, n := 1, i+1, tn
			it := &mediafiles.Item{TitleID: ti.ID, PartID: &p.ID, Kind: "track", Numbers: mediafiles.Numbers{Disc: &disc, Track: &num}, Name: &n, KeepUpToDate: true, SourceIDs: map[string]string{}}
			if _, err := database.Insert(tx, "items", it, domain.System); err != nil {
				return err
			}
			sl := &mediafiles.Slot{ItemID: it.ID, VersionID: v.ID, State: mediafiles.SlotWanted}
			if _, err := database.Insert(tx, "slots", sl, domain.System); err != nil {
				return err
			}
			slots = append(slots, sl.ID)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc, s, lf, partID, slots
}

// A folder that names no album is left alone by a scan, and a person links it
// to the album by hand: each of its files becomes the file of its track and
// nothing on disk moves.
func TestLinkAlbumFolderByHand(t *testing.T) {
	svc, s, lf, partID, slots := albumFolderWorld(t)
	ctx := context.Background()
	// The folder of an album HolyArrs would never guess: its name is not the album's.
	folder := filepath.Join(lf.Path, "Charlotte Gainsbourg", "Unsorted 2006")
	for _, f := range []string{"01 - AF607105.mp3", "02 - 5_55.mp3", "03 - The Operation.mp3"} {
		id3File(t, filepath.Join(folder, f), 100)
	}
	if _, err := svc.Rescan(ctx, domain.System); err != nil {
		t.Fatal(err)
	}
	if linked := linkedPaths(t, s); len(linked) != 0 {
		t.Fatalf("a scan linked %d files of a folder that names no album; the test needs that to be 0", len(linked))
	}
	out, err := svc.LinkAlbumFolder(ctx, partID, "Unsorted 2006", domain.System)
	if err != nil {
		t.Fatalf("linking the album folder by hand: %v", err)
	}
	if out.Files != 3 || out.Linked != 3 {
		t.Fatalf("files %d, linked %d; want 3 and 3 (left alone: %v)", out.Files, out.Linked, out.Unknown)
	}
	linked := linkedPaths(t, s)
	for i, sl := range slots {
		if _, ok := linked[sl]; !ok {
			t.Errorf("track %d not linked", i+1)
		}
	}
}

// A folder that is not there, or not one folder, is refused.
func TestLinkAlbumFolderRefusals(t *testing.T) {
	svc, _, _, partID, _ := albumFolderWorld(t)
	ctx := context.Background()
	for _, folder := range []string{"", ".", "..", "No Such Folder", "a/b"} {
		if _, err := svc.LinkAlbumFolder(ctx, partID, folder, domain.System); !errors.Is(err, ErrBadAlbumFolder) {
			t.Errorf("folder %q: got %v, want ErrBadAlbumFolder", folder, err)
		}
	}
}
