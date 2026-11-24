package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// writeVideo writes a small file at p, making its folders.
func writeVideo(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// versionFiles lists the files of the title's main (fourK false) or 4K
// Version, by their path in the Library folder.
func versionFiles(t *testing.T, l *moveLab, fourK bool) []string {
	t.Helper()
	q := "SELECT f.relative_path FROM files f JOIN versions v ON v.id = f.version_id WHERE v.is_main = 1 AND f.state = 'present'"
	if fourK {
		q = "SELECT f.relative_path FROM files f JOIN versions v ON v.id = f.version_id WHERE v.four_k = 1 AND v.is_main = 0 AND f.state = 'present'"
	}
	rows, err := l.st.Read().Query(q)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var rel string
		if err := rows.Scan(&rel); err != nil {
			t.Fatal(err)
		}
		out = append(out, filepath.ToSlash(rel))
	}
	return out
}

// TestMoveFourKFallbackNeverTakesATrailer: the 4K app's file is gone and
// its folder holds only the movie's trailer, named as media servers name
// one with dots, so no file is linked to the 4K Version.
func TestMoveFourKFallbackNeverTakesATrailer(t *testing.T) {
	l, fourKFile, _ := fourKLab(t, false)
	if err := os.Remove(fourKFile); err != nil {
		t.Fatal(err)
	}
	writeVideo(t, filepath.Join(filepath.Dir(fourKFile), "The.Matrix.1999.trailer.mkv"), "a trailer")
	mv := l.move(t, Choices{})
	if mv.Report == nil {
		t.Fatalf("move %+v", mv)
	}
	if got := versionFiles(t, l, true); len(got) != 0 {
		t.Fatalf("the 4K Version took %v: %+v", got, lineOf(t, mv.Report, "title:movie:tmdb:603"))
	}
	if got := lineOf(t, mv.Report, "title:movie:tmdb:603"); !strings.Contains(got.Detail, "no file was linked") {
		t.Fatalf("title line %+v", got)
	}
}

// TestMoveFourKFallbackSeesEveryCut: the 4K app's file is gone and its
// folder holds two cuts with thousands of extras in a subfolder between
// them; the extras never hide the second cut, so neither is linked.
func TestMoveFourKFallbackSeesEveryCut(t *testing.T) {
	l, fourKFile, _ := fourKLab(t, false)
	dir := filepath.Dir(fourKFile)
	if err := os.Remove(fourKFile); err != nil {
		t.Fatal(err)
	}
	writeVideo(t, filepath.Join(dir, "A.mkv"), "one cut, the bigger")
	writeVideo(t, filepath.Join(dir, "Z.mkv"), "another cut")
	for i := 0; i < 4999; i++ {
		writeVideo(t, filepath.Join(dir, "Featurettes", fmt.Sprintf("extra-%04d.mkv", i)), "x")
	}
	mv := l.move(t, Choices{})
	if mv.Report == nil {
		t.Fatalf("move %+v", mv)
	}
	if got := versionFiles(t, l, true); len(got) != 0 {
		t.Fatalf("the 4K Version took %v of two cuts", got)
	}
}

// TestMoveFourKRecordedFileInASubfolder: the 4K app recorded a file in a
// subfolder of the movie's folder, as Radarr scans them; that file is
// linked, never another file of the same name directly in the folder.
func TestMoveFourKRecordedFileInASubfolder(t *testing.T) {
	l, fourKFile, _ := fourKLabWith(t, false, "", "Feature.mkv", "The Matrix (1999)", nil, func(uhd *Snapshot) {
		uhd.Titles[0].Files = []TitleFile{{Name: "Feature.mkv", Path: "Disc1/Feature.mkv", Quality: "Remux-2160p"}}
	})
	writeVideo(t, filepath.Join(filepath.Dir(fourKFile), "Disc1", "Feature.mkv"), "the recorded 4K file")
	mv := l.move(t, Choices{})
	if mv.Report == nil {
		t.Fatalf("move %+v", mv)
	}
	if got := versionFiles(t, l, true); len(got) != 1 || got[0] != "The Matrix (1999)/Disc1/Feature.mkv" {
		t.Fatalf("the 4K Version took %v: %+v", got, lineOf(t, mv.Report, "title:movie:tmdb:603"))
	}
}

// TestMoveFourKRecordedSubfolderFileGone: the file the 4K app recorded in
// a subfolder is gone; a file of the same name directly in the folder is
// another one, so with a second cut beside it nothing is linked.
func TestMoveFourKRecordedSubfolderFileGone(t *testing.T) {
	l, fourKFile, _ := fourKLabWith(t, false, "", "Feature.mkv", "The Matrix (1999)", nil, func(uhd *Snapshot) {
		uhd.Titles[0].Files = []TitleFile{{Name: "Feature.mkv", Path: "Disc1/Feature.mkv", Quality: "Remux-2160p"}}
	})
	writeVideo(t, filepath.Join(filepath.Dir(fourKFile), "Other.Cut.mkv"), "another cut")
	mv := l.move(t, Choices{})
	if mv.Report == nil {
		t.Fatalf("move %+v", mv)
	}
	if got := versionFiles(t, l, true); len(got) != 0 {
		t.Fatalf("the 4K Version took %v", got)
	}
}

// TestMoveMainRecordedFileInASubfolder: the 1080p app recorded a file in a
// subfolder; the main copy adopts that file, not the bigger one directly
// in the folder.
func TestMoveMainRecordedFileInASubfolder(t *testing.T) {
	l, _, _ := fourKLabWith(t, false, "", "The.Matrix.1999.2160p.mkv", "The Matrix (1999)", func(radarr *Snapshot) {
		radarr.Titles[0].Files = []TitleFile{{Name: "Main.mkv", Path: "Disc1/Main.mkv", Quality: "Bluray-1080p"}}
	}, nil)
	writeVideo(t, filepath.Join(filepath.Dir(l.file), "Disc1", "Main.mkv"), "x")
	mv := l.move(t, Choices{})
	if got := versionFiles(t, l, false); len(got) != 1 || got[0] != "The Matrix (1999)/Disc1/Main.mkv" {
		t.Fatalf("the main Version took %v: %+v", got, lineOf(t, mv.Report, "title:movie:tmdb:603"))
	}
}

// TestRadarrMovieFileInASubfolderKeepsItsPath: a movie file Radarr found
// in a subfolder keeps its path through the read, with forward slashes,
// with or without a quality, and a merged movie's plan names it so.
func TestRadarrMovieFileInASubfolderKeepsItsPath(t *testing.T) {
	u, key := radarrAPIWith(t, map[string]any{"/api/v3/movie": []any{
		map[string]any{"id": 1, "tmdbId": 603, "title": "The Matrix", "year": 1999, "path": "/movies/The Matrix (1999)",
			"monitored": true, "qualityProfileId": 1, "movieFile": map[string]any{"relativePath": `Disc1\Feature.mkv`,
				"quality": map[string]any{"quality": map[string]any{"name": "Bluray-1080p"}}}},
		map[string]any{"id": 2, "tmdbId": 604, "title": "Unwatched", "year": 2003, "path": "/movies/Unwatched (2003)",
			"monitored": true, "qualityProfileId": 1, "movieFile": map[string]any{"relativePath": "Disc1/Unwatched.mkv", "quality": map[string]any{}}},
	}})
	s := read(t, api(Radarr, u, key))
	if f := findTitle(t, s, "The Matrix").Files; len(f) != 1 || f[0].Name != "Feature.mkv" || f[0].Path != "Disc1/Feature.mkv" || f[0].Quality != "Bluray-1080p" {
		t.Fatalf("files %+v", f)
	}
	if f := findTitle(t, s, "Unwatched").Files; len(f) != 1 || f[0].Name != "Unwatched.mkv" || f[0].Path != "Disc1/Unwatched.mkv" {
		t.Fatalf("files without a quality %+v", f)
	}
	uhd := radarrOneMovie("/movies4k", "Remux-2160p", true)
	if tp := item(t, BuildPlan([]Input{{1, s}, {2, uhd}}, Choices{}), GroupTitles, "title:movie:tmdb:603").Title; tp.FourK == nil || tp.File != "Disc1/Feature.mkv" {
		t.Fatalf("merged %v, plan file %q", tp.FourK != nil, tp.File)
	}
}

// TestMoveExistingFourKMainVersionKeepsItsQuality: a title already here
// was made from its 4K copy alone, so its main Version is a 4K one in the
// 4K app's folder. It keeps the 4K app's quality, never the 1080p app's for
// a file of the same name, and the line does not ask to add the 4K copy
// it already has.
func TestMoveExistingFourKMainVersionKeepsItsQuality(t *testing.T) {
	l, _, fourKLib := fourKLabNamed(t, false, "The Matrix (1999).mkv", "The Matrix (1999).mkv")
	ctx := context.Background()
	by := domain.Actor{Kind: "user", ID: 1}
	lf, err := library.AddFolder(ctx, l.st, library.FolderInput{Path: fourKLib, Kind: "movie", Label: "Movies 4K"}, nil, by)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.mv.Library.Adopt(ctx, lf.ID, []library.AdoptChoice{{Folder: "The Matrix (1999)", TMDB: "603"}}, by); err != nil {
		t.Fatal(err)
	}
	if err := l.st.Write(ctx, func(tx *sql.Tx) error { _, e := tx.Exec("UPDATE versions SET four_k = 1"); return e }); err != nil {
		t.Fatal(err)
	}
	mv := l.move(t, Choices{})
	if mv.Report == nil {
		t.Fatalf("move %+v", mv)
	}
	got := lineOf(t, mv.Report, "title:movie:tmdb:603")
	if got.Result != ResAlready || strings.Contains(got.Detail, "add its 4K copy") {
		t.Fatalf("title line %+v", got)
	}
	var q string
	if err := l.st.Read().QueryRow("SELECT COALESCE(json_extract(media_info, '$.recorded_quality'), '') FROM files").Scan(&q); err != nil {
		t.Fatal(err)
	}
	if q != "Remux-2160p" {
		t.Fatalf("the 4K copy's file has the quality %q", q)
	}
}

// TestMoveFourKCopyKeptUpToDateBesideALookOnlyMain: the main copy's
// Library folder is look-only and the 1080p app did not keep the movie up
// to date, but the 4K app did. The title and its movie item are kept up to
// date, so a search finds the 4K copy, with the main copy switched off; the
// line says so instead of "Keep up to date off".
func TestMoveFourKCopyKeptUpToDateBesideALookOnlyMain(t *testing.T) {
	l, _, _ := fourKLab(t, false)
	if _, err := library.AddFolder(context.Background(), l.st, library.FolderInput{Path: l.movies, Kind: "movie", Label: "Movies", LookOnly: true}, nil, domain.System); err != nil {
		t.Fatal(err)
	}
	mv := l.move(t, Choices{})
	if mv.Report == nil {
		t.Fatalf("move %+v", mv)
	}
	got := lineOf(t, mv.Report, "title:movie:tmdb:603")
	if !strings.Contains(got.Detail, fourKOnlyKept) || strings.Contains(got.Detail, "Keep up to date off") {
		t.Fatalf("title line %+v", got)
	}
	for q, want := range map[string]int{
		"SELECT count(*) FROM titles WHERE state != 'removed' AND keep_up_to_date = 1":                                         1,
		"SELECT count(*) FROM items i JOIN titles t ON t.id = i.title_id WHERE t.state != 'removed' AND i.keep_up_to_date = 1": 1,
		"SELECT count(*) FROM versions WHERE four_k = 1 AND is_main = 0 AND keep_up_to_date = 1":                               1,
		"SELECT count(*) FROM versions WHERE is_main = 1 AND keep_up_to_date = 1":                                              0,
	} {
		if n := l.count(t, q); n != want {
			t.Fatalf("%s: %d, want %d", q, n, want)
		}
	}
}

// TestMoveDryRunLeavesOutFoldersOfAPartThatIsOff: a show Library folder
// planned first holds the 4K Library folder, but Shows is off, so the run
// does not add it and the 4K one comes over; the dry run promises the same.
func TestMoveDryRunLeavesOutFoldersOfAPartThatIsOff(t *testing.T) {
	l, _, fourKLib := fourKLab(t, false)
	readBeforeFourK(t, l, &Snapshot{Kind: Sonarr, Version: "4.0.16"}, fourKLib)
	l.parts["shows"] = true
	by := domain.Actor{Kind: "user", ID: 1}
	dry := l.mv.Run(context.Background(), 0, l.plan(t), true, by, nil)
	if dl := lineOf(t, dry, "title:movie:tmdb:603"); !strings.Contains(dl.Detail, "its 4K copy becomes a 4K Version") {
		t.Fatalf("dry run %+v", dl)
	}
	mv := l.move(t, Choices{})
	if mv.Report == nil {
		t.Fatalf("move %+v", mv)
	}
	if n := l.count(t, "SELECT count(*) FROM versions WHERE four_k = 1"); n != 1 {
		t.Fatalf("%d 4K Versions, want 1: %+v", n, lineOf(t, mv.Report, "title:movie:tmdb:603"))
	}
}

// TestMoveDryRunFallbackTakesAFolderThisRunAdds: the main copy's Library
// folder is not on this server, a look-only movie Library folder is here,
// and the run adds the writable 4K one. The real run lands the main copy
// in the writable folder, where the 4K copy's folder is, so the 4K copy is
// not brought; the dry run says the same instead of promising it.
func TestMoveDryRunFallbackTakesAFolderThisRunAdds(t *testing.T) {
	l, _, _ := fourKLab(t, false)
	if err := os.RemoveAll(l.movies); err != nil {
		t.Fatal(err)
	}
	lookOnly := filepath.Join(t.TempDir(), "look")
	if err := os.MkdirAll(lookOnly, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := library.AddFolder(context.Background(), l.st, library.FolderInput{Path: lookOnly, Kind: "movie", Label: "Look", LookOnly: true}, nil, domain.System); err != nil {
		t.Fatal(err)
	}
	by := domain.Actor{Kind: "user", ID: 1}
	dry := l.mv.Run(context.Background(), 0, l.plan(t), true, by, nil)
	if dl := lineOf(t, dry, "title:movie:tmdb:603"); !strings.Contains(dl.Detail, "a 4K Version needs a folder of its own") {
		t.Fatalf("dry run %+v", dl)
	}
	mv := l.move(t, Choices{})
	if mv.Report == nil {
		t.Fatalf("move %+v", mv)
	}
	if n := l.count(t, "SELECT count(*) FROM versions v JOIN titles t ON t.id = v.title_id WHERE t.state != 'removed'"); n != 1 {
		t.Fatalf("%d Versions, want 1: %+v", n, lineOf(t, mv.Report, "title:movie:tmdb:603"))
	}
}

// TestMoveFourKCopySearchableBesideALookOnlyMainKeptUpToDate: both apps
// keep the movie up to date, but the main copy's Library folder is
// look-only, which leaves the movie's item off; it is turned on, so a
// search finds the 4K copy, and the main copy stays off.
func TestMoveFourKCopySearchableBesideALookOnlyMainKeptUpToDate(t *testing.T) {
	l, _, _ := fourKLabWith(t, false, "", "The.Matrix.1999.2160p.mkv", "The Matrix (1999)", func(radarr *Snapshot) {
		radarr.Titles[0].Monitored = true
	}, nil)
	if _, err := library.AddFolder(context.Background(), l.st, library.FolderInput{Path: l.movies, Kind: "movie", Label: "Movies", LookOnly: true}, nil, domain.System); err != nil {
		t.Fatal(err)
	}
	mv := l.move(t, Choices{})
	if mv.Report == nil {
		t.Fatalf("move %+v", mv)
	}
	if got := lineOf(t, mv.Report, "title:movie:tmdb:603"); strings.Contains(got.Detail, "Keep up to date") {
		t.Fatalf("title line %+v", got)
	}
	for q, want := range map[string]int{
		"SELECT count(*) FROM titles WHERE state != 'removed' AND keep_up_to_date = 1":                                         1,
		"SELECT count(*) FROM items i JOIN titles t ON t.id = i.title_id WHERE t.state != 'removed' AND i.keep_up_to_date = 1": 1,
		"SELECT count(*) FROM versions WHERE four_k = 1 AND is_main = 0 AND keep_up_to_date = 1":                               1,
		"SELECT count(*) FROM versions WHERE is_main = 1 AND keep_up_to_date = 1":                                              0,
	} {
		if n := l.count(t, q); n != want {
			t.Fatalf("%s: %d, want %d", q, n, want)
		}
	}
}
