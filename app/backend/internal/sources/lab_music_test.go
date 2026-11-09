//go:build lab

package sources

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// TestLab_AlbumSearch checks a 12-track album is picked as one Release against the lab indexer.
func TestLab_AlbumSearch(t *testing.T) {
	f := seed(t, labURL())
	var verID, partID int64
	var slots []int64
	err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		var pr, lf int64
		if err := tx.QueryRow("SELECT id FROM pick_rules WHERE name = ?", mediafiles.LosslessFirstName).Scan(&pr); err != nil {
			return err
		}
		if err := tx.QueryRow("SELECT id FROM library_folders LIMIT 1").Scan(&lf); err != nil {
			return err
		}
		ti := &mediafiles.Title{Kind: "artist", GateDriver: "none", Name: "Lab Lanterns", SortName: "lab lanterns", Aliases: []string{},
			SourceIDs: map[string]string{"musicbrainz": "lab"}, MetadataSource: "musicbrainz", KeepUpToDate: true, NewPartsPolicy: "all",
			ReleaseGate: "released", MusicTypes: []string{"album"}, WhyAdded: "by_hand", Artwork: []string{}, Status: "running", State: "active"}
		if _, err := database.Insert(tx, "titles", ti, domain.System); err != nil {
			return err
		}
		v := &mediafiles.Version{TitleID: ti.ID, Label: "Main", PickRulesID: pr, LibraryFolderID: lf, FolderName: "Lab Lanterns", KeepUpToDate: true, IsMain: true}
		if _, err := database.Insert(tx, "versions", v, domain.System); err != nil {
			return err
		}
		verID = v.ID
		date := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
		p := &mediafiles.Part{TitleID: ti.ID, Kind: "album", Name: "Harbour Light", ReleaseType: ptr("album"), ReleaseDate: &date, KeepUpToDate: true,
			Included: true, SourceIDs: map[string]string{}, Pressings: []mediafiles.Pressing{}, SecondaryTypes: []string{}}
		if _, err := database.Insert(tx, "parts", p, domain.System); err != nil {
			return err
		}
		partID = p.ID
		for k := 1; k <= 12; k++ {
			it := &mediafiles.Item{TitleID: ti.ID, PartID: &p.ID, Kind: "track", Numbers: mediafiles.Numbers{Disc: ptr(1), Track: ptr(k)},
				Name: ptr(fmt.Sprintf("Song %d", k)), AirDate: &date, LengthMS: ptr(4 * 60_000), KeepUpToDate: true, SourceIDs: map[string]string{}}
			if _, err := database.Insert(tx, "items", it, domain.System); err != nil {
				return err
			}
			sl := &mediafiles.Slot{ItemID: it.ID, VersionID: v.ID, State: mediafiles.SlotWanted}
			if _, err := database.Insert(tx, "slots", sl, domain.System); err != nil {
				return err
			}
			slots = append(slots, sl.ID)
		}
		_, err := tx.Exec(`UPDATE sources SET kinds = '[]', categories = '{"movie":[2000],"tv":[5000],"music":[3000]}' WHERE id = ?`, f.srcID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var picks int
	sr := &Searcher{Store: f.s, OnChosen: func(context.Context, int64, int64) { picks++ }}
	j := runKind(t, f, sr, jobs.KindSearchAlbum, &domain.Subject{Type: "part", ID: partID}, map[string]any{"version_id": verID})
	if j.State != "done" {
		t.Fatalf("job %s %+v", j.State, j.Error)
	}
	for _, id := range slots {
		var st string
		_ = f.s.Read().QueryRow("SELECT state FROM slots WHERE id = ?", id).Scan(&st)
		if st != string(mediafiles.SlotChosen) {
			t.Fatalf("slot %d is %s", id, st)
		}
	}
	win := relByName(t, f, "Lab Lanterns - Harbour Light (2024) [FLAC]")
	if win.State != "chosen" || len(PickSlots(win)) != 12 || picks != 1 {
		t.Fatalf("winner %s %v picks %d %v", win.State, PickSlots(win), picks, reasonCodes(win))
	}
	if r := relByName(t, f, "Lab Lanterns - Harbour Light (2024) [MP3 320]"); !hasCode(r, "rank.lost") {
		t.Fatalf("mp3 %s %v", r.State, reasonCodes(r))
	}
	if r := relByName(t, f, "Lab Lanterns - Harbour Light (2024) [FLAC] (image+cue)"); r.State != "refused" || !hasCode(r, "hard.never:single_image") {
		t.Fatalf("image %s %v", r.State, reasonCodes(r))
	}
	if r := relByName(t, f, "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS"); !hasCode(r, "match.none") {
		t.Fatalf("movie in an album search %v", reasonCodes(r))
	}
	t.Logf("chosen %s for %d tracks", win.Name, len(PickSlots(win)))
}
