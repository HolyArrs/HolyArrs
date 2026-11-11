package shows

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Sonarr names files after its own series title, such as "The Office (US)",
// while the details source calls the show "The Office": a moved show's
// episodes must still be recognised in its own folder (found in the B7 lab
// move, where all six files of The Office were left alone).
func TestMapEpisodeFilesTakesTheFolderShowsOtherNames(t *testing.T) {
	sh := metadata.Show{Name: "The Office", Year: 2005, OtherNames: []string{"The Office (U.S.)", "The Office US"},
		Episodes: []metadata.Episode{{Season: 1, Number: 1}, {Season: 1, Number: 2}}}
	vids := []library.AdoptVideo{{Path: "Season 1/The Office (US) - S01E01 - Pilot.mkv", Size: 1 << 20},
		{Path: "Season 1/The Office (US) - S01E02 - Diversity Day.mkv", Size: 1 << 20}}
	for _, f := range mapEpisodeFiles(vids, sh, "aired") {
		if len(f.Episodes) != 1 {
			t.Fatalf("%s: not matched (%s)", f.Path, f.Why)
		}
	}
	// In the show's own folder a file under another name still links by
	// its numbers, as Sonarr does.
	other := mapEpisodeFiles([]library.AdoptVideo{{Path: "Parks and Recreation - S01E01.mkv", Size: 1 << 20}}, sh, "aired")
	if len(other[0].Episodes) != 1 {
		t.Fatalf("a file in the show's folder was left alone: %+v", other[0])
	}
}
