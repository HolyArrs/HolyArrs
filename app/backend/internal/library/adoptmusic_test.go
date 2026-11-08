package library

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func TestAlbumFolderName(t *testing.T) {
	for in, want := range map[string]string{
		"Old Days (2010)":            "Old Days|2010",
		"2010 - Old Days":            "Old Days|2010",
		"[2010] Old Days":            "Old Days|2010",
		"Lab Band - Old Days (2010)": "Old Days|2010",
		"lab band - Old Days":        "Old Days|0",
		"Old Days":                   "Old Days|0",
		"Lab Bandwagon (2001)":       "Lab Bandwagon|2001",
	} {
		n, y := albumFolderName(in, "Lab Band")
		if got := fmt.Sprintf("%s|%d", n, y); got != want {
			t.Errorf("%q: %q want %q", in, got, want)
		}
	}
}

// labMusicFolder lays out a music Library folder: the lab artist with two
// albums (Old Days as the four-track deluxe on two discs, Middle named
// "Artist - Album"), a loose file, an album folder that is no album, and a
// folder of an unknown artist.
func labMusicFolder(t *testing.T, root string) {
	t.Helper()
	for _, f := range []string{
		"Lab Band/Old Days (2010)/CD1/01 - Song 1.flac", "Lab Band/Old Days (2010)/CD1/02 - Song 2.flac",
		"Lab Band/Old Days (2010)/CD1/03 - Song 3.flac", "Lab Band/Old Days (2010)/CD2/01 - Song 4.flac",
		"Lab Band/Lab Band - Middle (2018)/01 - Song m1.mp3", "Lab Band/Lab Band - Middle (2018)/02 - Song m2.mp3",
		"Lab Band/Lab Band - Middle (2018)/cover.jpg",
		"Lab Band/Unreleased Demos/demo.mp3",
		"Lab Band/intro.mp3",
		"Nobody Known/Thing (2001)/01 - A.mp3",
	} {
		writeFile(t, filepath.Join(root, filepath.FromSlash(f)), "audio:"+f)
	}
}

// 4.15.2: the dry run matches artist folders to artists, album folders to
// albums and Pressings, and files to tracks, with confidence; it writes
// nothing.
func TestPlanMusicAdopt(t *testing.T) {
	svc, s, fm, lf := musicService(t)
	ctx := context.Background()
	labMusicFolder(t, lf.Path)
	var before int
	_ = s.Read().QueryRow("SELECT count(*) FROM titles").Scan(&before)
	p, err := PlanMusicAdoptFrom(ctx, s.Read(), fm, lf.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	var after int
	_ = s.Read().QueryRow("SELECT count(*) FROM titles").Scan(&after)
	if after != before {
		t.Fatal("the dry run wrote a title")
	}
	if p.Kind != "music" || len(p.Folders) != 2 || p.Counts["matched"] != 1 || p.Counts["unmatched"] != 1 {
		t.Fatalf("plan %+v", p)
	}
	c := p.Folders[0]
	if c.Folder != "Lab Band" || c.Match == nil || c.Match.ID != "mbid:"+labArtist || c.Match.Score != 1 || c.Status != "matched" {
		t.Fatalf("artist %+v", c)
	}
	if c.Tracks != 6 || c.Unknown != 2 || len(c.Albums) != 3 || len(c.Files) != 1 || !strings.Contains(c.Files[0].Why, "not in an album folder") {
		t.Fatalf("counts %+v", c)
	}
	byFolder := map[string]AdoptAlbum{}
	for _, al := range c.Albums {
		byFolder[al.Folder] = al
	}
	od := byFolder["Old Days (2010)"]
	if od.Status != "matched" || od.Match.ID != rgOld || od.Tracks != 4 || od.Matched != 4 || !strings.Contains(od.Pressing, "4 tracks") {
		t.Fatalf("old days %+v", od)
	}
	want := map[string]string{"Lab Band/Old Days (2010)/CD2/01 - Song 4.flac": "disc 2 track 01 Song 4", "Lab Band/Old Days (2010)/CD1/03 - Song 3.flac": "disc 1 track 03 Song 3"}
	for _, f := range od.Files {
		if w, ok := want[f.Path]; ok && f.Track != w {
			t.Errorf("%s: %q", f.Path, f.Track)
		}
	}
	if m := byFolder["Lab Band - Middle (2018)"]; m.Status != "matched" || m.Match.ID != rgMid || m.Matched != 2 {
		t.Fatalf("middle %+v", m)
	}
	if u := byFolder["Unreleased Demos"]; u.Status != "unmatched" || u.Unknown != 1 {
		t.Fatalf("demos %+v", u)
	}
	if p.Folders[1].Folder != "Nobody Known" || p.Folders[1].Status != "unmatched" {
		t.Fatalf("unknown artist %+v", p.Folders[1])
	}
	_ = svc
}

// 4.15.2: adopting keeps every file where it is: the artist takes the
// folder as it is, the deluxe Pressing the files fit is chosen, and each
// file becomes an adopted File of its track.
func TestAdoptMusicMovesNothing(t *testing.T) {
	svc, s, _, lf := musicService(t)
	ctx := context.Background()
	labMusicFolder(t, lf.Path)
	before := snapshot(t, lf.Path)
	res, err := svc.Adopt(ctx, lf.ID, []AdoptChoice{{Folder: "Lab Band", ID: "mbid:" + labArtist}}, domain.System)
	if err != nil || len(res) != 1 || res[0].Error != "" || res[0].TitleID == 0 {
		t.Fatalf("%+v %v", res, err)
	}
	if res[0].Episodes != 6 || len(res[0].Unknown) != 2 {
		t.Fatalf("result %+v", res[0])
	}
	if fmt.Sprint(snapshot(t, lf.Path)) != fmt.Sprint(before) {
		t.Fatal("adopting moved or changed a file")
	}
	id := res[0].TitleID
	var folder string
	_ = s.Read().QueryRow("SELECT folder_name FROM versions WHERE title_id = ?", id).Scan(&folder)
	if folder != "Lab Band" {
		t.Fatalf("folder %q", folder)
	}
	var chosen string
	_ = s.Read().QueryRow("SELECT mbid FROM pressings WHERE chosen = 1 AND part_id IN (SELECT id FROM parts WHERE title_id = ? AND json_extract(source_ids, '$.musicbrainz') = ?)", id, rgOld).Scan(&chosen)
	if chosen != relOldDx {
		t.Fatalf("pressing %s", chosen)
	}
	files, _ := database.Select[mediafiles.File](ctx, s.Read(), "files", "version_id IN (SELECT id FROM versions WHERE title_id = ?) ORDER BY relative_path", id)
	if len(files) != 6 {
		t.Fatalf("files %d", len(files))
	}
	for _, f := range files {
		if f.Origin != "adopted" || len(f.SlotIDs) != 1 || f.State != "present" {
			t.Fatalf("file %+v", f)
		}
		var state string
		_ = s.Read().QueryRow("SELECT state FROM slots WHERE id = ?", f.SlotIDs[0]).Scan(&state)
		if state != string(mediafiles.SlotInLibrary) && state != string(mediafiles.SlotBetterWanted) {
			t.Fatalf("slot %s", state)
		}
	}
	// Adopting the same folder again is refused.
	res, _ = svc.Adopt(ctx, lf.ID, []AdoptChoice{{Folder: "Lab Band", ID: "mbid:" + labArtist}}, domain.System)
	if res[0].Error == "" {
		t.Fatal("adopted twice")
	}
	for _, bad := range []string{"../x", "Lab Band/Old Days (2010)", ".holding", "Missing"} {
		res, _ = svc.Adopt(ctx, lf.ID, []AdoptChoice{{Folder: bad, ID: "mbid:" + labArtist}}, domain.System)
		if res[0].Error == "" {
			t.Fatalf("%q adopted", bad)
		}
	}
	// The scan now lists the folder as in the Library.
	p, _ := PlanMusicAdoptFrom(ctx, s.Read(), svc.Artists, lf.ID, "")
	if p.Folders[0].Status != "in_library" {
		t.Fatalf("rescan %+v", p.Folders[0])
	}
}

// A rescan marks a lost track file missing and a returned one present,
// records an unknown audio file as found, and a dropped mount (most files
// gone at once) marks nothing.
func TestRescanMusic(t *testing.T) {
	svc, s, _, lf := musicService(t)
	ctx := context.Background()
	labMusicFolder(t, lf.Path)
	res, _ := svc.Adopt(ctx, lf.ID, []AdoptChoice{{Folder: "Lab Band", ID: "mbid:" + labArtist}}, domain.System)
	id := res[0].TitleID
	gone := filepath.Join(lf.Path, "Lab Band", "Lab Band - Middle (2018)", "01 - Song m1.mp3")
	data, _ := os.ReadFile(gone)
	_ = os.Remove(gone)
	writeFile(t, filepath.Join(lf.Path, "Lab Band", "Old Days (2010)", "bonus.mp3"), "bonus")
	r, err := svc.Rescan(ctx, domain.System)
	if err != nil || r.Missing != 1 || r.Found < 1 {
		t.Fatalf("%+v %v", r, err)
	}
	var origin string
	_ = s.Read().QueryRow("SELECT origin FROM files WHERE relative_path = ?", "Lab Band/Old Days (2010)/bonus.mp3").Scan(&origin)
	if origin != "found" {
		t.Fatalf("bonus %q", origin)
	}
	_ = os.WriteFile(gone, data, 0o644)
	if r, _ := svc.Rescan(ctx, domain.System); r.Back != 1 {
		t.Fatalf("back %+v", r)
	}
	// Every known file gone at once while the Library folder still holds
	// something: a half-gone mount, nothing marked missing.
	_ = os.RemoveAll(filepath.Join(lf.Path, "Lab Band"))
	r, _ = svc.Rescan(ctx, domain.System)
	if r.Missing != 0 || r.Unreachable != 1 {
		t.Fatalf("mount guard %+v", r)
	}
	var missing int
	_ = s.Read().QueryRow("SELECT count(*) FROM files WHERE state = 'missing' AND version_id IN (SELECT id FROM versions WHERE title_id = ?)", id).Scan(&missing)
	if missing != 0 {
		t.Fatalf("%d marked missing", missing)
	}
}

// An empty music Library folder that should hold files is a warning in the
// scan (a mount that dropped).
func TestPlanMusicAdoptMountWarning(t *testing.T) {
	svc, s, fm, lf := musicService(t)
	ctx := context.Background()
	labMusicFolder(t, lf.Path)
	if _, err := svc.Adopt(ctx, lf.ID, []AdoptChoice{{Folder: "Lab Band", ID: "mbid:" + labArtist}}, domain.System); err != nil {
		t.Fatal(err)
	}
	ents, _ := os.ReadDir(lf.Path)
	for _, e := range ents {
		_ = os.RemoveAll(filepath.Join(lf.Path, e.Name()))
	}
	p, err := PlanMusicAdoptFrom(ctx, s.Read(), fm, lf.ID, "")
	if err != nil || !strings.Contains(p.Warning, "not mounted") {
		t.Fatalf("%q %v", p.Warning, err)
	}
	if _, err := PlanMusicAdoptFrom(ctx, s.Read(), nil, lf.ID, ""); err == nil {
		t.Fatal("no MusicBrainz: scanned")
	}
}

// TestLabMusicAdoptedWithNoFileMoved adopts a copy of the lab music folder
// (4.15.2 done-when). The lab folder itself is only read. Run with
//
//	HOLYARRS_LAB_MUSIC=<holyarrs>/research/lab/media/music go test -run LabMusic ./internal/library/
func TestLabMusicAdoptedWithNoFileMoved(t *testing.T) {
	src := os.Getenv("HOLYARRS_LAB_MUSIC")
	if src == "" {
		t.Skip("HOLYARRS_LAB_MUSIC not set")
	}
	labBefore := snapshot(t, src)
	svc, s, fm, lf := musicService(t)
	copyTree(t, src, lf.Path)
	before := snapshot(t, lf.Path)
	// A fake MusicBrainz that knows each lab artist and album by its
	// folder's name, with one Pressing per album holding the folder's tracks.
	fm.artists = map[string]metadata.Artist{}
	fm.pressings = map[string][]metadata.Pressing{}
	ents, _ := os.ReadDir(lf.Path)
	for i, e := range ents {
		labFakeArtist(t, fm, lf.Path, e.Name(), i)
	}
	ctx := context.Background()
	plan, err := PlanMusicAdoptFrom(ctx, s.Read(), fm, lf.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	var cs []AdoptChoice
	for _, c := range plan.Folders {
		if c.Status != "matched" || c.Unknown != 0 {
			t.Errorf("%s: %s (%s), %d left alone", c.Folder, c.Status, c.Why, c.Unknown)
			continue
		}
		cs = append(cs, AdoptChoice{Folder: c.Folder, ID: c.Match.ID})
	}
	res, err := svc.Adopt(ctx, lf.ID, cs, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	tracks := 0
	for _, r := range res {
		if r.TitleID == 0 || r.Error != "" || len(r.Unknown) != 0 {
			t.Errorf("%s: %+v", r.Folder, r)
		}
		tracks += r.Episodes
	}
	if fmt.Sprint(snapshot(t, lf.Path)) != fmt.Sprint(before) {
		t.Fatal("adopting moved or changed a file")
	}
	if fmt.Sprint(snapshot(t, src)) != fmt.Sprint(labBefore) {
		t.Fatal("the lab folder changed")
	}
	t.Logf("adopted %d lab artists, %d tracks, %d files untouched", len(res), tracks, len(before))
}

// labFakeArtist teaches the fake MusicBrainz one lab artist folder: the
// artist by the folder name, an album per album folder by its name and
// year, and one Pressing per album with a track per "NN - Title" file.
func labFakeArtist(t *testing.T, fm *fakeMusic, root, folder string, i int) {
	t.Helper()
	a := metadata.Artist{MBID: fmt.Sprintf("b%07d-0000-0000-0000-000000000000", i), Name: folder, Type: "group", Status: "running"}
	albums, _ := os.ReadDir(filepath.Join(root, folder))
	for j, al := range albums {
		if !al.IsDir() {
			continue
		}
		name, year := albumFolderName(al.Name(), folder)
		rg := fmt.Sprintf("c%07d-0000-0000-0000-%012d", i, j)
		a.Albums = append(a.Albums, metadata.Album{MBID: rg, Name: name, PrimaryType: "album", ReleaseDate: fmt.Sprintf("%04d-01-01", max(year, 1900))})
		files, _ := os.ReadDir(filepath.Join(root, folder, al.Name()))
		var trs []metadata.Track
		for _, f := range files {
			base := strings.TrimSuffix(f.Name(), filepath.Ext(f.Name()))
			var n int
			var title string
			if _, err := fmt.Sscanf(base, "%d", &n); err != nil {
				continue
			}
			_, title, _ = strings.Cut(base, " - ")
			trs = append(trs, metadata.Track{ID: fmt.Sprintf("%s-t%d", rg, n), RecordingID: fmt.Sprintf("%s-r%d", rg, n), Disc: 1, Number: n, Position: n, Name: title})
		}
		fm.pressings[rg] = []metadata.Pressing{{MBID: fmt.Sprintf("d%07d-0000-0000-0000-%012d", i, j), Name: name, Status: "official", Date: fmt.Sprint(year),
			Formats: []string{"Digital Media"}, Tracks: trs}}
	}
	fm.artists[a.MBID] = a
}

// An album folder of an album the album kinds leave out (a single) is
// listed as not included in the dry run and left alone when adopting.
func TestMusicAdoptAlbumNotIncluded(t *testing.T) {
	svc, s, fm, lf := musicService(t)
	ctx := context.Background()
	writeFile(t, filepath.Join(lf.Path, "Lab Band", "One Song (2019)", "01 - Song s.mp3"), "x")
	writeFile(t, filepath.Join(lf.Path, "Lab Band", "Middle (2018)", "01 - Song m1.mp3"), "y")
	p, err := PlanMusicAdoptFrom(ctx, s.Read(), fm, lf.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	var single AdoptAlbum
	for _, al := range p.Folders[0].Albums {
		if al.Folder == "One Song (2019)" {
			single = al
		}
	}
	if single.Status != "not_included" || single.Unknown != 1 || single.Match == nil || single.Match.ID != rgSingle {
		t.Fatalf("%+v", single)
	}
	res, _ := svc.Adopt(ctx, lf.ID, []AdoptChoice{{Folder: "Lab Band", ID: "mbid:" + labArtist}}, domain.System)
	if res[0].Episodes != 1 || len(res[0].Unknown) != 1 || !strings.Contains(res[0].Unknown[0], "not included") {
		t.Fatalf("%+v", res[0])
	}
}
