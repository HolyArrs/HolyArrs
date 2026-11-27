package importer

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// beerus is the show the report is about: a fansub group names its files after
// the arc alone ("[Judas] Beerus - S01E05.mkv") or shortens the show to "DBS".
// The download was grabbed for this show and these Slots, so the show comes
// from the grab and only the episode from the file, as Sonarr does.
func beerusTarget(t *testing.T) ShowTarget {
	t.Helper()
	ns := mediafiles.NamingDefaults()
	ns.RenameOnImport = true
	return ShowTarget{LibraryPath: "/lib", FolderName: "Dragon Ball Super Beerus",
		Show:   mediafiles.TitleRef{Name: "Dragon Ball Super: Beerus", Aliases: []string{"Dragon Ball Super: Beerus (Dub)"}},
		Order:  mediafiles.OrderAired,
		Naming: &ns,
		Episodes: []Episode{
			{Ref: mediafiles.EpisodeRef{ItemID: 5, Season: 1, Episode: 5}, Name: "Five", SlotID: 15, Wanted: true},
		},
		ReleaseName: "[Judas] Dragon Ball Super  - Beerus - S01E05 [1080p][HEVC x265 10bit][Multi-Subs] (Weekly)"}
}

// The group's short name for the show is the show under another spelling, so
// the grab still vouches for it: the arc alone, the group's letters for the
// whole show, or no name at all.
func TestPlanTakesShowFromGrabWhenFileShortensItsName(t *testing.T) {
	for _, name := range []string{
		"[Judas] Beerus - S01E05.mkv", // the arc alone, as reported
		"[Judas] DBS - S01E05.mkv",    // the group's shortening of the show
		"[Judas] - S01E05.mkv",        // no name at all
		"[Judas] Beerus - S01E05 [1080p][HEVC x265 10bit][Multi-Subs] (Weekly).mkv",
		"[Judas] Beerus S01E05 [1080p].mkv",
	} {
		s := PlanEpisodes([]Candidate{{Path: "/dl/x/" + name, Size: 1}}, beerusTarget(t))
		if s[0].Method != "hardlink" || len(s[0].TargetSlotIDs) != 1 || s[0].TargetSlotIDs[0] != 15 || !strings.Contains(s[0].TargetPath, "S01E05") {
			t.Errorf("%s: %s %v", name, s[0].Method, s[0].Why)
		}
	}
}

// Another show's name or letters are not this show's short name, and a
// download with no grab still needs the file to name the show.
func TestPlanShortNameIsNotAnotherShow(t *testing.T) {
	for _, name := range []string{"[Judas] One Piece - S01E05.mkv", "[Judas] OP - S01E05.mkv", "[Judas] Weekly - S01E05.mkv"} {
		if s := PlanEpisodes([]Candidate{{Path: "/dl/x/" + name, Size: 1}}, beerusTarget(t)); s[0].Method == "hardlink" {
			t.Errorf("%s was placed: %+v", name, s[0])
		}
	}
	tgt := beerusTarget(t)
	tgt.ReleaseName = ""
	if s := PlanEpisodes([]Candidate{{Path: "/dl/x/[Judas] DBS - S01E05.mkv", Size: 1}}, tgt); s[0].Method == "hardlink" {
		t.Errorf("no grab: %+v", s[0])
	}
}

// A file the grab vouches for still has to give an episode the show wants.
func TestPlanGrabDoesNotPlaceOtherNumbers(t *testing.T) {
	if s := PlanEpisodes([]Candidate{{Path: "/dl/x/[Judas] DBS - S01E09.mkv", Size: 1}}, beerusTarget(t)); s[0].Method == "hardlink" {
		t.Errorf("an episode the show does not want was placed: %+v", s[0])
	}
}

// The whole way through, as the reported download arrives: the group leaves the
// show's name off the only file inside, and it still lands in its Slot.
func TestShowImportOfGrabbedDownloadWithoutSeriesPrefix(t *testing.T) {
	w := showSetup(t, []int{3}, "[Judas] Beerus - S01E03.mkv")
	rel := "[Judas] Dragon Ball Super  - Beerus - S01E03 [1080p][HEVC x265 10bit][Multi-Subs] (Weekly)"
	if err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE titles SET name = ?", "Dragon Ball Super: Beerus"); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE releases SET name = ? WHERE id = ?", rel, w.relID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	imp, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System)
	if err != nil || imp.State != "done" {
		t.Fatalf("import: %v %+v", err, imp)
	}
	if f := importedFile(t, w, 3); !strings.Contains(f.RelativePath, "S01E03") {
		t.Fatalf("placed as %s", f.RelativePath)
	}
}
