package shows

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// copperShow is a show of seasons x episodes; ids are per (season, number)
// so a renumbered episode is a new episode at the source.
func copperShow(seasons, per int, extra ...metadata.Episode) metadata.Show {
	sh := metadata.Show{Source: "tmdb", Name: "Copper Valley", Year: 2020, Status: "ended", IDs: map[string]string{"tmdb": "700", "tvdb": "701"}}
	for se := 1; se <= seasons; se++ {
		sh.Seasons = append(sh.Seasons, metadata.Season{Number: se, Name: fmt.Sprintf("Season %d", se)})
		for e := 1; e <= per; e++ {
			sh.Episodes = append(sh.Episodes, metadata.Episode{Season: se, Number: e, Name: fmt.Sprintf("Ep %d", e), AirDate: tm("2020-01-06T02:00:00Z"), AirDay: "2020-01-05",
				ID: fmt.Sprintf("tmdb-%d-%d", se, e)})
		}
	}
	for _, e := range extra {
		if e.Season == 0 && len(sh.Seasons) > 0 && sh.Seasons[0].Number != 0 {
			sh.Seasons = append([]metadata.Season{{Number: 0, Name: "Specials"}}, sh.Seasons...)
		}
		sh.Episodes = append(sh.Episodes, e)
	}
	return sh
}

// adoptCopper writes a 2x12 folder, adopts it and returns the title id.
func adoptCopper(t *testing.T, svc *Service, tmdb *fakeShows, lf mediafiles.LibraryFolder) int64 {
	t.Helper()
	tmdb.set(copperShow(2, 12), "tmdb:700", "tvdb:701")
	for se := 1; se <= 2; se++ {
		for e := 1; e <= 12; e++ {
			writeFile(t, filepath.Join(lf.Path, "Copper Valley (2020)", fmt.Sprintf("Season %d", se), fmt.Sprintf("Copper.Valley.S%02dE%02d.1080p.WEB.mkv", se, e)), strings.Repeat("x", 100+se*20+e))
		}
	}
	res, err := svc.Adopt(context.Background(), lf.ID, []library.AdoptChoice{{Folder: "Copper Valley (2020)", ID: "tmdb:700"}}, domain.System)
	if err != nil || res[0].Error != "" || res[0].Episodes != 24 {
		t.Fatalf("adopt %+v %v", res, err)
	}
	return res[0].TitleID
}

func leftovers(t *testing.T, svc *Service, id int64) map[string]library.LeftoverView {
	t.Helper()
	ls, err := svc.Leftovers(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]library.LeftoverView{}
	for _, l := range ls {
		out[filepath.Base(l.RelativePath)] = l
	}
	return out
}

func filled(t *testing.T, s *database.Store, id int64) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for k, e := range episodes(t, s, id) {
		var fid *int64
		_ = s.Read().QueryRow("SELECT file_id FROM slots WHERE id = ?", e.SlotID).Scan(&fid)
		out[k] = fid != nil && !e.Dropped
	}
	return out
}

// Leftovers test 1 and 4: the source merges 2x12 into 1x24. After a refresh and
// rescan the twelve season 2 files are leftovers; a bulk match links all
// 24 episodes, moves nothing, and survives a refresh and a rescan. When the
// episodes leave the list again the files are leftovers again.
func TestLeftoversAfterSeasonMerge(t *testing.T) {
	svc, s, tmdb, _ := addShowService(t)
	ctx := context.Background()
	lf := addShowFolder(t, s)
	id := adoptCopper(t, svc, tmdb, lf)
	before := snapshot(t, lf.Path)

	tmdb.set(copperShow(1, 24), "tmdb:700", "tvdb:701")
	if _, err := svc.RefreshShow(ctx, id, domain.System, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RescanTitle(ctx, id, domain.System); err != nil {
		t.Fatal(err)
	}
	ls := leftovers(t, svc, id)
	if len(ls) != 12 {
		t.Fatalf("leftovers %d: %+v", len(ls), ls)
	}
	for e := 1; e <= 12; e++ {
		l, ok := ls[fmt.Sprintf("Copper.Valley.S02E%02d.1080p.WEB.mkv", e)]
		if !ok || l.Reason != "episode_not_listed" || l.DroppedSeason == nil || *l.DroppedSeason != 2 || l.Guess == nil || l.Guess.Season != 2 || !reflect.DeepEqual(l.Guess.Episodes, []int{e}) {
			t.Fatalf("S02E%02d %+v", e, l)
		}
	}
	if f := filled(t, s, id); f["S01E13"] || !f["S01E12"] {
		t.Fatalf("before the match: %v", f)
	}

	eps := episodes(t, s, id)
	var items []library.LeftoverMatch
	for e := 1; e <= 12; e++ {
		items = append(items, library.LeftoverMatch{FileID: ls[fmt.Sprintf("Copper.Valley.S02E%02d.1080p.WEB.mkv", e)].FileID, EpisodeIDs: []int64{eps[fmt.Sprintf("S01E%02d", e+12)].ItemID}})
	}
	out, err := svc.MatchLeftovers(ctx, id, items, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range out {
		if !o.Linked {
			t.Fatalf("not linked %+v", out)
		}
	}
	check := func(when string) {
		t.Helper()
		f := filled(t, s, id)
		for e := 1; e <= 24; e++ {
			if !f[fmt.Sprintf("S01E%02d", e)] {
				t.Fatalf("%s: S01E%02d has no file", when, e)
			}
		}
		if n := len(leftovers(t, svc, id)); n != 0 {
			t.Fatalf("%s: %d leftovers", when, n)
		}
		if !reflect.DeepEqual(snapshot(t, lf.Path), before) {
			t.Fatalf("%s: a file moved", when)
		}
	}
	check("after the match")
	for k, e := range episodes(t, s, id) {
		if e.Dropped {
			t.Errorf("dropped episode %s left behind", k)
		}
	}
	if _, err := svc.RefreshShow(ctx, id, domain.System, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Rescan(ctx, domain.System); err != nil {
		t.Fatal(err)
	}
	check("after a refresh and a rescan")

	// The source splits the season again: the hand-matched files hold
	// episodes it no longer lists, so they are leftovers again.
	tmdb.set(copperShow(1, 12), "tmdb:700", "tvdb:701")
	if _, err := svc.RefreshShow(ctx, id, domain.System, nil); err != nil {
		t.Fatal(err)
	}
	if n := len(leftovers(t, svc, id)); n != 12 {
		t.Fatalf("after the episodes left again: %d leftovers", n)
	}
}

// Leftovers test 2 and 7: an episode removed while it holds a file lists the
// file with its media facts; unmatch restores a note and dismiss lasts.
func TestLeftoverRemovedEpisodeUnmatchDismiss(t *testing.T) {
	svc, s, tmdb, _ := addShowService(t)
	ctx := context.Background()
	lf := addShowFolder(t, s)
	id := adoptCopper(t, svc, tmdb, lf)
	sh := copperShow(2, 12)
	sh.Episodes = sh.Episodes[:len(sh.Episodes)-1] // S02E12 is gone
	tmdb.set(sh, "tmdb:700", "tvdb:701")
	if _, err := svc.RefreshShow(ctx, id, domain.System, nil); err != nil {
		t.Fatal(err)
	}
	ls := leftovers(t, svc, id)
	l, ok := ls["Copper.Valley.S02E12.1080p.WEB.mkv"]
	if len(ls) != 1 || !ok || l.Reason != "episode_not_listed" || l.Resolution != 1080 {
		t.Fatalf("leftovers %+v", ls)
	}
	var f mediafiles.File
	if err := database.Get(ctx, s.Read(), "files", l.FileID, &f); err != nil || len(f.MediaInfo) == 0 {
		t.Fatalf("media facts lost: %+v %v", f.MediaInfo, err)
	}

	// Unmatch a good file: it becomes a note; matching it back fills it.
	eps := episodes(t, s, id)
	one := eps["S01E01"]
	var fid int64
	_ = s.Read().QueryRow("SELECT file_id FROM slots WHERE id = ?", one.SlotID).Scan(&fid)
	if n, err := svc.UnmatchLeftovers(ctx, id, []int64{fid}, domain.System); err != nil || n != 1 {
		t.Fatalf("unmatch %d %v", n, err)
	}
	if l := leftovers(t, svc, id)["Copper.Valley.S01E01.1080p.WEB.mkv"]; l.Reason != "unmatched" {
		t.Fatalf("unmatched %+v", l)
	}
	if err := svc.DismissLeftover(ctx, id, fid, true); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Rescan(ctx, domain.System); err != nil {
		t.Fatal(err)
	}
	// The rescan links the note back to its empty episode; dismiss only
	// matters while it stays a leftover.
	if f := filled(t, s, id); !f["S01E01"] {
		t.Fatal("the rescan did not link the note back")
	}
	if err := svc.DismissLeftover(ctx, id, l.FileID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Rescan(ctx, domain.System); err != nil {
		t.Fatal(err)
	}
	if l := leftovers(t, svc, id)["Copper.Valley.S02E12.1080p.WEB.mkv"]; !l.Dismissed {
		t.Fatalf("dismiss did not last a rescan: %+v", l)
	}
	if err := svc.DismissLeftover(ctx, id+99, l.FileID, false); err == nil {
		t.Error("dismissed a file of another title")
	}
}

// Leftovers test 5 and 6: the match checks, replace, a double-episode file, a
// special.
func TestLeftoverMatchChecks(t *testing.T) {
	svc, s, tmdb, _ := addShowService(t)
	ctx := context.Background()
	lf := addShowFolder(t, s)
	id := adoptCopper(t, svc, tmdb, lf)
	tmdb.set(copperShow(2, 12, metadata.Episode{Season: 0, Number: 1, Name: "Making of", ID: "tmdb-0-1"}), "tmdb:700", "tvdb:701")
	if _, err := svc.RefreshShow(ctx, id, domain.System, nil); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(lf.Path, "Copper Valley (2020)")
	writeFile(t, filepath.Join(dir, "Extras", "making of copper valley.mkv"), "m")
	writeFile(t, filepath.Join(dir, "Extras", "copper valley double.mkv"), "dd")
	before := snapshot(t, lf.Path)
	if _, err := svc.RescanTitle(ctx, id, domain.System); err != nil {
		t.Fatal(err)
	}
	ls := leftovers(t, svc, id)
	mk, dbl := ls["making of copper valley.mkv"], ls["copper valley double.mkv"]
	if mk.Reason != "no_episode_numbers" || dbl.FileID == 0 {
		t.Fatalf("leftovers %+v", ls)
	}
	eps := episodes(t, s, id)
	other := addShowService2(t, svc, tmdb, lf)
	before = snapshot(t, lf.Path)
	ep := func(k string) []int64 { return []int64{eps[k].ItemID} }
	cases := []struct {
		item library.LeftoverMatch
		want string
	}{
		{library.LeftoverMatch{FileID: mk.FileID, EpisodeIDs: []int64{other}}, "wrong_title"},
		{library.LeftoverMatch{FileID: mk.FileID, EpisodeIDs: ep("S01E01")}, "episode_has_file"},
		{library.LeftoverMatch{FileID: mk.FileID}, "no_episode"},
		{library.LeftoverMatch{FileID: 999999, EpisodeIDs: ep("S00E01")}, "not_a_leftover"},
	}
	for _, c := range cases {
		out, err := svc.MatchLeftovers(ctx, id, []library.LeftoverMatch{c.item}, false, domain.System)
		if err != nil || out[0].Linked || out[0].Refused != c.want {
			t.Errorf("%+v: %+v %v, want %s", c.item, out, err, c.want)
		}
	}
	// A busy episode is refused.
	if err := s.Write(ctx, execTx("UPDATE slots SET state = 'downloading' WHERE id = ?", eps["S00E01"].SlotID)); err != nil {
		t.Fatal(err)
	}
	if out, _ := svc.MatchLeftovers(ctx, id, []library.LeftoverMatch{{FileID: mk.FileID, EpisodeIDs: ep("S00E01")}}, false, domain.System); out[0].Refused != "episode_busy" {
		t.Errorf("busy %+v", out)
	}
	if err := s.Write(ctx, execTx("UPDATE slots SET state = 'wanted' WHERE id = ?", eps["S00E01"].SlotID)); err != nil {
		t.Fatal(err)
	}
	// A special into season 0.
	if out, _ := svc.MatchLeftovers(ctx, id, []library.LeftoverMatch{{FileID: mk.FileID, EpisodeIDs: ep("S00E01")}}, false, domain.System); !out[0].Linked {
		t.Errorf("special %+v", out)
	}
	// Replace: one file over two episodes that have files; the old ones
	// become notes and nothing is deleted.
	out, _ := svc.MatchLeftovers(ctx, id, []library.LeftoverMatch{{FileID: dbl.FileID, EpisodeIDs: append(ep("S01E01"), ep("S01E02")...)}}, true, domain.System)
	if !out[0].Linked {
		t.Fatalf("replace %+v", out)
	}
	var slots string
	_ = s.Read().QueryRow("SELECT slot_ids FROM files WHERE id = ?", dbl.FileID).Scan(&slots)
	if slots != fmt.Sprintf("[%d,%d]", eps["S01E01"].SlotID, eps["S01E02"].SlotID) {
		t.Errorf("double-episode slots %s", slots)
	}
	ls = leftovers(t, svc, id)
	if ls["Copper.Valley.S01E01.1080p.WEB.mkv"].Reason != "replaced_by_hand" || ls["Copper.Valley.S01E02.1080p.WEB.mkv"].Reason != "replaced_by_hand" {
		t.Errorf("replaced files %+v", ls)
	}
	if !reflect.DeepEqual(snapshot(t, lf.Path), before) {
		t.Error("a match changed the disk")
	}
}

// addShowService2 adds a second show and returns one of its episode ids.
func addShowService2(t *testing.T, svc *Service, tmdb *fakeShows, lf mediafiles.LibraryFolder) int64 {
	t.Helper()
	sh := copperShow(1, 2)
	sh.Name, sh.IDs = "Iron Hills", map[string]string{"tmdb": "800"}
	for i := range sh.Episodes {
		sh.Episodes[i].ID = fmt.Sprintf("iron-%d", i)
	}
	tmdb.set(sh, "tmdb:800")
	writeFile(t, filepath.Join(lf.Path, "Iron Hills", "Iron.Hills.S01E01.mkv"), "i")
	res, err := svc.Adopt(context.Background(), lf.ID, []library.AdoptChoice{{Folder: "Iron Hills", ID: "tmdb:800"}}, domain.System)
	if err != nil || res[0].TitleID == 0 {
		t.Fatalf("%+v %v", res, err)
	}
	return episodes(t, svc.Store, res[0].TitleID)["S01E02"].ItemID
}

// Leftovers test 3: a title rescan with its folder gone, or most files gone at
// once, marks nothing missing.
func TestRescanTitleGuards(t *testing.T) {
	svc, s, tmdb, _ := addShowService(t)
	ctx := context.Background()
	lf := addShowFolder(t, s)
	id := adoptCopper(t, svc, tmdb, lf)
	if err := os.RemoveAll(filepath.Join(lf.Path, "Copper Valley (2020)")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(lf.Path, "keep.txt"), "k")
	r, err := svc.RescanTitle(ctx, id, domain.System)
	if err != nil || r.Missing != 0 || r.Unreachable != 1 {
		t.Fatalf("folder gone %+v %v", r, err)
	}
	_ = os.Remove(filepath.Join(lf.Path, "keep.txt"))
	if r, _ := svc.RescanTitle(ctx, id, domain.System); r.Missing != 0 || r.Unreachable != 1 {
		t.Fatalf("library folder empty %+v", r)
	}
	var missing int
	_ = s.Read().QueryRow("SELECT count(*) FROM files WHERE state = 'missing'").Scan(&missing)
	if missing != 0 {
		t.Fatalf("%d files marked missing", missing)
	}
}

func execTx(q string, args ...any) func(*sql.Tx) error {
	return func(tx *sql.Tx) error { _, err := tx.Exec(q, args...); return err }
}

// A folder with files of a season the source does not list: adopting shows
// them as leftovers at once, with no rescan in between.
func TestAdoptRecordsLeftovers(t *testing.T) {
	svc, s, tmdb, _ := addShowService(t)
	lf := addShowFolder(t, s)
	tmdb.set(copperShow(1, 5), "tmdb:700", "tvdb:701")
	for _, n := range []string{"S01E01", "S01E02", "S01E03", "S01E04", "S01E05", "S02E01", "S02E02"} {
		writeFile(t, filepath.Join(lf.Path, "Copper Valley (2020)", "Season "+n[2:3], "Copper.Valley."+n+".1080p.WEB.mkv"), strings.Repeat("x", 100))
	}
	res, err := svc.Adopt(context.Background(), lf.ID, []library.AdoptChoice{{Folder: "Copper Valley (2020)", ID: "tmdb:700"}}, domain.System)
	if err != nil || res[0].Error != "" || res[0].Episodes != 5 || len(res[0].Unknown) != 2 {
		t.Fatalf("adopt %+v %v", res, err)
	}
	ls := leftovers(t, svc, res[0].TitleID)
	if len(ls) != 2 {
		t.Fatalf("leftovers %d: %+v", len(ls), ls)
	}
	for _, n := range []string{"S02E01", "S02E02"} {
		l, ok := ls["Copper.Valley."+n+".1080p.WEB.mkv"]
		if !ok || l.Reason != "episode_not_listed" || l.Guess == nil || l.Guess.Season != 2 {
			t.Fatalf("%s %+v", n, l)
		}
	}
	if f := filled(t, s, res[0].TitleID); !f["S01E05"] {
		t.Fatalf("filled %v", f)
	}
}
