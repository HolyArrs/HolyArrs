package library

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// an album named "5:55" in a "5_55" folder, with or without a
// format tag, gets its files on a scan.
func TestReconcileForbiddenCharAlbum(t *testing.T) {
	for _, folder := range []string{"5_55", "5_55 (2006) [FLAC 24-96]", "5_55 [Japan]"} {
		t.Run(folder, func(t *testing.T) {
			svc, s, _, lf := musicService(t)
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
				p := &mediafiles.Part{TitleID: ti.ID, Kind: "album", Name: "5:55", ReleaseDate: &rd, KeepUpToDate: true, SourceIDs: map[string]string{"musicbrainz": "f1000000-0000-4000-8000-000000000001"},
					Pressings: []mediafiles.Pressing{}, SecondaryTypes: []string{}, Included: true}
				if _, err := database.Insert(tx, "parts", p, domain.System); err != nil {
					return err
				}
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
			for _, f := range []string{"01 - AF607105.mp3", "02 - 5_55.mp3", "03 - The Operation.mp3"} {
				id3File(t, filepath.Join(lf.Path, "Charlotte Gainsbourg", folder, f), 100)
			}
			if _, err := svc.Rescan(context.Background(), domain.System); err != nil {
				t.Fatal(err)
			}
			linked := linkedPaths(t, s)
			for i, sl := range slots {
				if _, ok := linked[sl]; !ok {
					t.Errorf("track %d not linked", i+1)
				}
			}
		})
	}
}
