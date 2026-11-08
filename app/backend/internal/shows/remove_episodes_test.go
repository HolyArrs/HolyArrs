package shows

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
)

// showDeleteWorld adopts the lab show tree (three adopted files: S01E01,
// S01E02 and the multi-episode S02E01E02) and records the files left alone
// as found by a rescan.
func showDeleteWorld(t *testing.T) (*Service, *database.Store, mediafiles.LibraryFolder, int64) {
	t.Helper()
	svc, s, _, _ := addShowService(t)
	ctx := context.Background()
	lf := addShowFolder(t, s)
	showTree(t, lf.Path)
	res, err := svc.Adopt(ctx, lf.ID, []library.AdoptChoice{{Folder: "Lab Show (2025)", ID: "tmdb:100"}}, domain.System)
	if err != nil || res[0].Error != "" {
		t.Fatalf("%+v %v", res, err)
	}
	if _, err := svc.Rescan(ctx, domain.System); err != nil {
		t.Fatal(err)
	}
	return svc, s, lf, res[0].TitleID
}

const multiFile = "Lab Show (2025)/Season 2/Lab.Show.S02E01E02.720p.HDTV.x264-GRP.mkv"

// 5.2.3: deleting a show keeps every file by default, ends its Slots and
// can put it on Never-add.
func TestDeleteShowKeepsFilesAndNeverAdd(t *testing.T) {
	svc, s, lf, id := showDeleteWorld(t)
	ctx := context.Background()
	before := snapshot(t, lf.Path)
	p, err := svc.Remove(ctx, id, library.RemoveChoice{NeverAdd: true}, true, domain.System)
	if err != nil || !p.Applied || p.Scope != "title" || len(p.Held) != 0 || len(p.Kept) != 6 {
		t.Fatalf("plan %+v %v", p, err)
	}
	if !reflect.DeepEqual(snapshot(t, lf.Path), before) {
		t.Fatal("a file moved")
	}
	for _, sl := range slotsOf(t, s, id) {
		if sl.State != mediafiles.SlotRemoved || sl.LastReason == nil || *sl.LastReason != "the show was deleted" {
			t.Fatalf("slot %+v", sl)
		}
	}
	if _, _, err := svc.AddShow(ctx, "tmdb:100", ShowAddOptions{}, domain.System); !errors.Is(err, ErrShowNeverAdd) {
		t.Fatalf("add again: %v", err)
	}
	if _, err := svc.Remove(ctx, id, library.RemoveChoice{}, false, domain.System); !errors.Is(err, library.ErrAlreadyRemoved) {
		t.Fatalf("again: %v", err)
	}
}

// 5.2.3: deleting a show with its files holds only the imported and
// adopted ones, exactly as the dry run said; found files and their
// folders stay.
func TestDeleteShowFilesDryRunIsApply(t *testing.T) {
	svc, s, lf, id := showDeleteWorld(t)
	ctx := context.Background()
	before := s.WriteCount()
	dry, err := svc.Remove(ctx, id, library.RemoveChoice{DeleteFiles: true}, false, domain.System)
	if err != nil || s.WriteCount() != before || dry.Applied {
		t.Fatalf("dry run %v", err)
	}
	if dry.HoldingDays != 7 || len(dry.Held) != 3 || len(dry.Kept) != 3 {
		t.Fatalf("dry %+v", dry)
	}
	p, err := svc.Remove(ctx, id, library.RemoveChoice{DeleteFiles: true}, true, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(dry.Held, p.Held) || !reflect.DeepEqual(dry.Kept, p.Kept) {
		t.Fatalf("dry %+v\napply %+v", dry, p)
	}
	for _, h := range p.Held {
		if onDisk(lf, h.Path) || !onDisk(lf, h.To) || !strings.HasPrefix(h.To, importer.HoldingDir+"/2026-10-13/") {
			t.Errorf("held %+v", h)
		}
		if h.Path == multiFile && !reflect.DeepEqual(h.Episodes, []string{"S02E01", "S02E02"}) {
			t.Errorf("multi episodes %v", h.Episodes)
		}
	}
	for _, k := range p.Kept {
		if k.Origin != "found" || !onDisk(lf, k.Path) {
			t.Errorf("kept %+v", k)
		}
	}
	// Season 2 held its only file and is gone; Season 1 keeps found files.
	if onDisk(lf, "Lab Show (2025)/Season 2/Lab.Show.S02E01E02.720p.HDTV.x264-GRP.mkv") || !onDisk(lf, "Lab Show (2025)/Season 1/S01E02.small.mkv") {
		t.Error("files")
	}
}

// 5.2.3: deleting a season holds its files (a multi-episode file whose
// episodes are all in it too), turns Keep up to date off for it, and
// leaves the other season alone.
func TestDeleteSeason(t *testing.T) {
	svc, s, lf, id := showDeleteWorld(t)
	ctx := context.Background()
	two := 2
	p, err := svc.RemoveEpisodes(ctx, id, library.RemoveScope{Season: &two}, library.RemoveChoice{DeleteFiles: true}, true, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if p.Scope != "season" || !reflect.DeepEqual(p.Episodes, []string{"S02E01", "S02E02", "S02E03"}) || len(p.Held) != 1 || p.Held[0].Path != multiFile {
		t.Fatalf("plan %+v", p)
	}
	if onDisk(lf, multiFile) || !onDisk(lf, "Lab Show (2025)/Season 1/Lab.Show.S01E01.1080p.WEB-DL.H264-GRP.mkv") {
		t.Fatal("files")
	}
	eps := episodes(t, s, id)
	for _, k := range []string{"S02E01", "S02E02"} {
		if e := eps[k]; e.Keep || e.State != mediafiles.SlotWanted || e.Reason != "its file was deleted" {
			t.Errorf("%s %+v", k, e)
		}
	}
	if e := eps["S01E01"]; !e.Keep || e.State == mediafiles.SlotWanted {
		t.Errorf("season 1 changed: %+v", e)
	}
	ss, _ := Seasons(ctx, s.Read(), id, now)
	for _, se := range ss {
		if se.Number == 2 && se.KeepUpToDate {
			t.Error("season 2 still kept up to date")
		}
	}
	var ti mediafiles.Title
	_ = database.Get(ctx, s.Read(), "titles", id, &ti)
	if ti.State == "removed" {
		t.Error("the show was deleted")
	}
	if es, _, _ := svc.Store.Events(ctx, database.EventFilter{Kinds: []string{domain.EventEpisodesRemoved}}); len(es) != 1 || es[0].Summary != "Deleted Lab Show season 2 from the Library" {
		t.Errorf("event %+v", es)
	}
}

// 5.2.3: one episode of a multi-episode file: the file stays for the
// other episode, and the episode stops being kept up to date.
func TestDeleteEpisodeOfMultiEpisodeFile(t *testing.T) {
	svc, s, lf, id := showDeleteWorld(t)
	ctx := context.Background()
	eps := episodes(t, s, id)
	p, err := svc.RemoveEpisodes(ctx, id, library.RemoveScope{EpisodeIDs: []int64{eps["S02E01"].ItemID}}, library.RemoveChoice{DeleteFiles: true}, true, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Held) != 0 || len(p.Kept) != 1 || p.Kept[0].Path != multiFile || !strings.Contains(p.Kept[0].Why, "also holds S02E02") {
		t.Fatalf("plan %+v", p)
	}
	if !onDisk(lf, multiFile) {
		t.Fatal("the multi-episode file moved")
	}
	after := episodes(t, s, id)
	if after["S02E01"].Keep || !after["S02E02"].Keep || after["S02E01"].State != eps["S02E01"].State {
		t.Errorf("after %+v %+v", after["S02E01"], after["S02E02"])
	}
	// Both of its episodes: now the file is held.
	p, err = svc.RemoveEpisodes(ctx, id, library.RemoveScope{EpisodeIDs: []int64{eps["S02E01"].ItemID, eps["S02E02"].ItemID}}, library.RemoveChoice{DeleteFiles: true}, true, domain.System)
	if err != nil || len(p.Held) != 1 || onDisk(lf, multiFile) {
		t.Fatalf("both %+v %v", p, err)
	}
}

// 5.2.3: refusals: Never-add only for a whole show, a season that does not
// exist, an episode of another show, nothing chosen, a movie.
func TestDeleteEpisodesRefusals(t *testing.T) {
	svc, s, _, id := showDeleteWorld(t)
	ctx := context.Background()
	one, nine := 1, 9
	cases := []struct {
		sc   library.RemoveScope
		c    library.RemoveChoice
		want error
	}{
		{library.RemoveScope{Season: &one}, library.RemoveChoice{NeverAdd: true}, library.ErrNeverAddWholeShow},
		{library.RemoveScope{Season: &nine}, library.RemoveChoice{}, library.ErrNoSuchSeason},
		{library.RemoveScope{EpisodeIDs: []int64{99999}}, library.RemoveChoice{}, library.ErrNoSuchEpisode},
		{library.RemoveScope{}, library.RemoveChoice{}, library.ErrNoEpisodesChosen},
	}
	for i, c := range cases {
		if _, err := svc.RemoveEpisodes(ctx, id, c.sc, c.c, true, domain.System); !errors.Is(err, c.want) {
			t.Errorf("case %d: %v, want %v", i, err, c.want)
		}
	}
	movie, err := library.AddMovie(ctx, s, dune, library.AddOptions{LibraryFolderID: addFolder(t, s).ID}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RemoveEpisodes(ctx, movie, library.RemoveScope{Season: &one}, library.RemoveChoice{}, false, domain.System); !errors.Is(err, library.ErrNotShow) {
		t.Errorf("movie: %v", err)
	}

}

// 5.2 review: a delete with files removes season folders it emptied (past
// empty folders nested in them) but never a person's own empty folders.
func TestDeleteRemovesOnlySeasonFoldersItEmptied(t *testing.T) {
	for _, season := range []bool{false, true} {
		svc, _, lf, id := showDeleteWorld(t)
		ctx := context.Background()
		show := filepath.Join(lf.Path, "Lab Show (2025)")
		if err := os.RemoveAll(filepath.Join(show, "Season 2", "Sample")); err != nil {
			t.Fatal(err)
		}
		for _, d := range []string{".actors", "Featurettes", filepath.Join("Season 2", "Subs", "en")} {
			if err := os.MkdirAll(filepath.Join(show, d), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		var err error
		if season {
			two := 2
			_, err = svc.RemoveEpisodes(ctx, id, library.RemoveScope{Season: &two}, library.RemoveChoice{DeleteFiles: true}, true, domain.System)
		} else {
			_, err = svc.Remove(ctx, id, library.RemoveChoice{DeleteFiles: true}, true, domain.System)
		}
		if err != nil {
			t.Fatal(err)
		}
		if onDisk(lf, "Lab Show (2025)/Season 2") {
			t.Errorf("season %v: the emptied season folder stayed", season)
		}
		if !onDisk(lf, "Lab Show (2025)/.actors") || !onDisk(lf, "Lab Show (2025)/Featurettes") {
			t.Errorf("season %v: a person's own empty folder went", season)
		}
		if !onDisk(lf, "Lab Show (2025)/Season 1") {
			t.Errorf("season %v: season 1 went", season)
		}
	}
}
