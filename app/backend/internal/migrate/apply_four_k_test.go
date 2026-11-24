package migrate

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// fourKLab is a move lab with two Radarrs: the lab's own read becomes the
// 1080p app, and a second read holds the same movie in a 4K Library
// folder. It returns the lab, the 4K media file and the 4K Library folder.
func fourKLab(t *testing.T, fourKFirst bool) (*moveLab, string, string) {
	t.Helper()
	return fourKLabNamed(t, fourKFirst, "", "The.Matrix.1999.2160p.mkv")
}

// fourKLabNamed is fourKLab with the files' names given; an empty main
// name keeps the lab's own.
func fourKLabNamed(t *testing.T, fourKFirst bool, mainName, fourKName string) (*moveLab, string, string) {
	t.Helper()
	return fourKLabIn(t, fourKFirst, mainName, fourKName, "The Matrix (1999)")
}

// fourKLabIn is fourKLabNamed with the 4K app's movie folder named too. An
// empty 4K file name makes a 4K copy not downloaded yet: no file and no
// folder.
func fourKLabIn(t *testing.T, fourKFirst bool, mainName, fourKName, fourKFolder string) (*moveLab, string, string) {
	t.Helper()
	return fourKLabWith(t, fourKFirst, mainName, fourKName, fourKFolder, nil, nil)
}

// fourKLabWith is fourKLabIn with editMain and editUHD, when set, changing
// the 1080p and the 4K app's reads before they are saved.
func fourKLabWith(t *testing.T, fourKFirst bool, mainName, fourKName, fourKFolder string, editMain, editUHD func(*Snapshot)) (*moveLab, string, string) {
	t.Helper()
	l := newMoveLabWith(t, func(radarr, _, _ *Snapshot) {
		radarr.QualityProfiles = []Profile{{ID: 2, Name: "HD-1080p", Cutoff: "Bluray-1080p"}}
		name := mainName
		if name == "" {
			name = "The.Matrix.1999.1080p.mkv"
		}
		radarr.Titles[0].Files = []TitleFile{{Name: name, Quality: "Bluray-1080p"}}
		if editMain != nil {
			editMain(radarr)
		}
	})
	if mainName != "" {
		renamed := filepath.Join(filepath.Dir(l.file), mainName)
		if err := os.Rename(l.file, renamed); err != nil {
			t.Fatal(err)
		}
		l.file = renamed
	}
	fourKLib := filepath.Join(t.TempDir(), "4k")
	fourKFile := filepath.Join(fourKLib, fourKFolder, fourKName)
	files := []TitleFile{{Name: fourKName, Quality: "Remux-2160p"}}
	if fourKName == "" {
		fourKFile, files = "", nil
		if err := os.MkdirAll(fourKLib, 0o755); err != nil {
			t.Fatal(err)
		}
	} else if err := os.MkdirAll(filepath.Dir(fourKFile), 0o755); err != nil {
		t.Fatal(err)
	} else if err := os.WriteFile(fourKFile, []byte("the 4K copy, bigger"), 0o644); err != nil {
		t.Fatal(err)
	}
	uhd := &Snapshot{Kind: Radarr, Via: ViaAPI,
		Folders:         []Folder{{Path: "/movies4k"}},
		QualityProfiles: []Profile{{ID: 2, Name: "Ultra-HD", Cutoff: "Remux-2160p"}},
		Titles: []Title{{SourceID: 1, Kind: "movie", Name: "The Matrix", Year: 1999, IDs: map[string]string{"tmdb": "603"},
			Path: "/movies4k/" + fourKFolder, Monitored: true, ProfileID: 2,
			Files: files}}}
	if editUHD != nil {
		editUHD(uhd)
	}
	uhd.finish()
	saved, err := Save(context.Background(), l.st, 1, uhd)
	if err != nil {
		t.Fatal(err)
	}
	if fourKFirst {
		l.snaps = append([]int64{saved.ID}, l.snaps...)
	} else {
		l.snaps = append(l.snaps, saved.ID)
	}
	l.rules = append(l.rules, PathRule{From: "/movies4k", To: filepath.ToSlash(fourKLib)})
	return l, fourKFile, fourKLib
}

func fileHash(t *testing.T, p string) [32]byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(b)
}

// TestMoveMergesFourKCopyAndUndoes: a movie in a 1080p and a 4K Radarr
// comes over as one title with a main and a 4K Version, each file adopted
// where it is; no file's bytes change, and undo takes the title and both
// Versions while both files stay on disk.
func TestMoveMergesFourKCopyAndUndoes(t *testing.T) {
	for _, fourKFirst := range []bool{false, true} {
		name := "1080p read first"
		if fourKFirst {
			name = "4K read first"
		}
		t.Run(name, func(t *testing.T) {
			l, fourKFile, fourKLib := fourKLab(t, fourKFirst)
			mainBefore, fourKBefore := fileHash(t, l.file), fileHash(t, fourKFile)
			// The plan merges the two reads into one title.
			it := item(t, l.plan(t), GroupTitles, "title:movie:tmdb:603")
			if it.Title.FourK == nil || it.Title.FourK.Library != filepath.ToSlash(fourKLib) {
				t.Fatalf("plan %+v", it.Title)
			}
			by := domain.Actor{Kind: "user", ID: 1}
			dry := l.mv.Run(context.Background(), 0, l.plan(t), true, by, nil)
			if dl := lineOf(t, dry, "title:movie:tmdb:603"); dl.Result != ResAdopt || !strings.Contains(dl.Detail, "its 4K copy becomes a 4K Version using the folder") {
				t.Fatalf("dry run %+v", dl)
			}
			mv := l.move(t, Choices{})
			if mv.State != "done" || mv.Report == nil {
				t.Fatalf("move %+v", mv)
			}
			got := lineOf(t, mv.Report, "title:movie:tmdb:603")
			if got.Result != ResAdopted || !strings.Contains(got.Detail, "its 4K copy came over as a 4K Version") {
				t.Fatalf("title line %+v", got)
			}
			if n := l.count(t, "SELECT count(*) FROM titles WHERE state != 'removed'"); n != 1 {
				t.Fatalf("%d titles", n)
			}
			// The 1080p app did not watch the movie but the 4K app did: the
			// title stays kept up to date with its main copy switched off,
			// so the 4K copy can still be upgraded.
			var keep bool
			if err := l.st.Read().QueryRow("SELECT keep_up_to_date FROM titles WHERE state != 'removed'").Scan(&keep); err != nil {
				t.Fatal(err)
			}
			if !keep {
				t.Fatal("the title is not kept up to date, so its watched 4K copy is never upgraded")
			}
			// One main Version on Standard, one 4K Version on the 4K Pick
			// rules in the 4K Library folder, kept up to date on its own.
			type ver struct {
				label, folder, lib, rules string
				fourK, isMain, keep       bool
			}
			vers := map[string]ver{}
			rows, err := l.st.Read().Query(`SELECT v.label, v.four_k, v.is_main, v.folder_name, lf.path, p.name, v.keep_up_to_date
				FROM versions v JOIN library_folders lf ON lf.id = v.library_folder_id JOIN pick_rules p ON p.id = v.pick_rules_id
				WHERE v.title_id IN (SELECT id FROM titles WHERE state != 'removed')`)
			if err != nil {
				t.Fatal(err)
			}
			for rows.Next() {
				var v ver
				if err := rows.Scan(&v.label, &v.fourK, &v.isMain, &v.folder, &v.lib, &v.rules, &v.keep); err != nil {
					rows.Close()
					t.Fatal(err)
				}
				vers[v.label] = v
			}
			rows.Close()
			if len(vers) != 2 {
				t.Fatalf("versions %+v", vers)
			}
			main, fourK := vers["Main"], vers["4K"]
			if !main.isMain || main.fourK || main.lib != l.movies || main.folder != "The Matrix (1999)" || main.rules != "Standard" || main.keep {
				t.Fatalf("main version %+v", main)
			}
			if fourK.isMain || !fourK.fourK || fourK.lib != fourKLib || fourK.folder != "The Matrix (1999)" || fourK.rules != "4K" || !fourK.keep {
				t.Fatalf("4K version %+v", fourK)
			}
			// Both files are adopted where they are, with the quality each
			// old app recorded.
			files := map[string][3]string{}
			rows, err = l.st.Read().Query(`SELECT v.label, f.relative_path, f.origin, f.state, COALESCE(json_extract(f.media_info, '$.recorded_quality'), '')
				FROM files f JOIN versions v ON v.id = f.version_id`)
			if err != nil {
				t.Fatal(err)
			}
			for rows.Next() {
				var label, rel, origin, state, quality string
				if err := rows.Scan(&label, &rel, &origin, &state, &quality); err != nil {
					rows.Close()
					t.Fatal(err)
				}
				files[label] = [3]string{rel, origin + "/" + state, quality}
			}
			rows.Close()
			if want := [3]string{"The Matrix (1999)/The.Matrix.1999.1080p.mkv", "adopted/present", "Bluray-1080p"}; files["Main"] != want {
				t.Fatalf("main file %+v, want %+v", files["Main"], want)
			}
			if want := [3]string{"The Matrix (1999)/The.Matrix.1999.2160p.mkv", "adopted/present", "Remux-2160p"}; files["4K"] != want {
				t.Fatalf("4K file %+v, want %+v", files["4K"], want)
			}
			if fileHash(t, l.file) != mainBefore || fileHash(t, fourKFile) != fourKBefore {
				t.Fatal("the move changed a media file's bytes")
			}
			// Undo takes the title and both Versions; both files stay.
			u := l.undo(t, mv.ID)
			if u.State != "undone" {
				t.Fatalf("undo %+v", u)
			}
			if ln := lineOf(t, u.Report, "title:movie:tmdb:603"); ln.Result != ResRemoved {
				t.Fatalf("undo title line %+v", ln)
			}
			if n := l.count(t, "SELECT count(*) FROM titles WHERE state != 'removed'"); n != 0 {
				t.Fatalf("%d titles after undo", n)
			}
			if n := l.count(t, "SELECT count(*) FROM versions v JOIN titles t ON t.id = v.title_id WHERE t.state != 'removed'"); n != 0 {
				t.Fatalf("%d versions after undo", n)
			}
			if fileHash(t, l.file) != mainBefore || fileHash(t, fourKFile) != fourKBefore {
				t.Fatal("undo changed a media file's bytes")
			}
		})
	}
}

// TestMoveFourKCopyOfTitleAlreadyHere: a title that was in the Library
// before the run gets no 4K Version; the line says to add it by hand.
func TestMoveFourKCopyOfTitleAlreadyHere(t *testing.T) {
	l, fourKFile, _ := fourKLab(t, false)
	before := fileHash(t, fourKFile)
	ctx := context.Background()
	by := domain.Actor{Kind: "user", ID: 1}
	lf, err := library.AddFolder(ctx, l.st, library.FolderInput{Path: l.movies, Kind: "movie"}, nil, by)
	if err != nil {
		t.Fatal(err)
	}
	m := l.mv.Library.Metadata.(*metadata.Static).Movies[0]
	if _, _, err := l.mv.Library.Add(ctx, m, library.AddOptions{LibraryFolderID: lf.ID, WhyAdded: "import"}, by); err != nil {
		t.Fatal(err)
	}
	mv := l.move(t, Choices{})
	if mv.State != "done" {
		t.Fatalf("move %+v", mv)
	}
	got := lineOf(t, mv.Report, "title:movie:tmdb:603")
	if got.Result != ResAlready || got.Detail != "already in the Library; add its 4K copy on the title page" {
		t.Fatalf("title line %+v", got)
	}
	if n := l.count(t, "SELECT count(*) FROM versions"); n != 1 {
		t.Fatalf("%d versions", n)
	}
	if n := l.count(t, "SELECT count(*) FROM versions WHERE four_k = 1"); n != 0 {
		t.Fatal("a 4K Version was added to a title that was already here")
	}
	if fileHash(t, fourKFile) != before {
		t.Fatal("the move changed the 4K file's bytes")
	}
}

// TestMoveFourKCopyMissingFolder: when the 4K Library folder is not on
// this server, the main title still comes over and the line says why the
// 4K copy was not brought.
func TestMoveFourKCopyMissingFolder(t *testing.T) {
	l, _, fourKLib := fourKLab(t, false)
	before := fileHash(t, l.file)
	if err := os.RemoveAll(fourKLib); err != nil {
		t.Fatal(err)
	}
	mv := l.move(t, Choices{})
	if mv.State != "done" {
		t.Fatalf("move %+v", mv)
	}
	got := lineOf(t, mv.Report, "title:movie:tmdb:603")
	if got.Result != ResAdopted || !strings.Contains(got.Detail, "its 4K copy was not brought: its Library folder") {
		t.Fatalf("title line %+v", got)
	}
	if n := l.count(t, "SELECT count(*) FROM versions"); n != 1 {
		t.Fatalf("%d versions", n)
	}
	if n := l.count(t, "SELECT count(*) FROM files WHERE origin = 'adopted'"); n != 1 {
		t.Fatalf("%d adopted files", n)
	}
	if fileHash(t, l.file) != before {
		t.Fatal("the move changed the main file's bytes")
	}
}

// TestMoveMergedTitleImportedOnce: a second run of the same import adds
// nothing twice, the merged title's two Versions included.
func TestMoveMergedTitleImportedOnce(t *testing.T) {
	l, _, _ := fourKLab(t, false)
	mv := l.move(t, Choices{})
	if mv.State != "done" {
		t.Fatalf("move %+v", mv)
	}
	var titleID int64
	if err := l.st.Read().QueryRow("SELECT id FROM titles WHERE state != 'removed'").Scan(&titleID); err != nil {
		t.Fatal(err)
	}
	again := l.move(t, Choices{})
	if got := lineOf(t, again.Report, "title:movie:tmdb:603"); got.Result != ResAlready {
		t.Fatalf("second move %+v", got)
	}
	if n := l.count(t, "SELECT count(*) FROM versions WHERE title_id = ?", titleID); n != 2 {
		t.Fatalf("%d versions after the second move", n)
	}
	if n := l.count(t, "SELECT count(*) FROM files"); n != 2 {
		t.Fatalf("%d files after the second move", n)
	}
}

// The two apps often name both copies alike. The quality each recorded
// stays on its own copy, also when the import runs again, so the 4K copy
// is never judged by the 1080p app's quality and replaced.
func TestFourKQualitiesStayOnTheirOwnCopy(t *testing.T) {
	l, _, _ := fourKLabNamed(t, false, "The Matrix (1999).mkv", "The Matrix (1999).mkv")
	quality := func() map[string]string {
		out := map[string]string{}
		rows, err := l.st.Read().Query(`SELECT v.label, COALESCE(json_extract(f.media_info, '$.recorded_quality'), '')
			FROM files f JOIN versions v ON v.id = f.version_id WHERE f.state = 'present'`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var label, q string
			_ = rows.Scan(&label, &q)
			out[label] = q
		}
		return out
	}
	for run := 1; run <= 2; run++ {
		if mv := l.move(t, Choices{}); mv.State != "done" {
			t.Fatalf("run %d: %+v", run, mv)
		}
		if q := quality(); q["Main"] != "Bluray-1080p" || q["4K"] != "Remux-2160p" {
			t.Fatalf("run %d: qualities %v", run, q)
		}
	}
}

// A 4K copy not downloaded yet has no folder, as Radarr keeps it: it comes
// over as a wanted 4K Version, and the dry run says so first.
func TestMoveWantedFourKCopyComesOverWithNoFile(t *testing.T) {
	l, fourKFile, _ := fourKLab(t, false)
	if err := os.RemoveAll(filepath.Dir(fourKFile)); err != nil {
		t.Fatal(err)
	}
	by := domain.Actor{Kind: "user", ID: 1}
	dry := l.mv.Run(context.Background(), 0, l.plan(t), true, by, nil)
	if dl := lineOf(t, dry, "title:movie:tmdb:603"); !strings.Contains(dl.Detail, "becomes a 4K Version with no file yet") {
		t.Fatalf("dry run %+v", dl)
	}
	mv := l.move(t, Choices{})
	if got := lineOf(t, mv.Report, "title:movie:tmdb:603"); !strings.Contains(got.Detail, "came over as a 4K Version with no file yet") {
		t.Fatalf("title line %+v", got)
	}
	if n := l.count(t, "SELECT count(*) FROM versions WHERE four_k = 1"); n != 1 {
		t.Fatalf("%d 4K Versions", n)
	}
	if n := l.count(t, "SELECT count(*) FROM files f JOIN versions v ON v.id = f.version_id WHERE v.four_k = 1"); n != 0 {
		t.Fatalf("%d files on the wanted 4K Version", n)
	}
}

// A 4K Library folder set to "Don't bring them" keeps its copies out.
func TestMoveLeftOutFourKFolderBringsNoFourKCopy(t *testing.T) {
	l, _, _ := fourKLab(t, false)
	mv := l.move(t, Choices{LeaveFolders: []string{"/movies4k"}})
	got := lineOf(t, mv.Report, "title:movie:tmdb:603")
	if !strings.Contains(got.Detail, "its 4K copy was left out") {
		t.Fatalf("title line %+v", got)
	}
	if n := l.count(t, "SELECT count(*) FROM versions WHERE four_k = 1"); n != 0 {
		t.Fatalf("%d 4K Versions", n)
	}
}

// registerRoots adds both Library folders before the import, as on a
// server that already has them.
func registerRoots(t *testing.T, l *moveLab, roots ...string) map[string]int64 {
	t.Helper()
	out := map[string]int64{}
	for _, p := range roots {
		lf, err := library.AddFolder(context.Background(), l.st, library.FolderInput{Path: p, Kind: "movie", Label: filepath.Base(p)}, nil, domain.System)
		if err != nil {
			t.Fatal(err)
		}
		out[p] = lf.ID
	}
	return out
}

// A different movie that already owns the folder the import would adopt is
// not this movie: the import refuses the title, gives the other movie no
// 4K copy, and undo leaves the other movie in the Library.
func TestMoveNeverTakesAnotherMoviesFolder(t *testing.T) {
	l, _, fourKLib := fourKLab(t, false)
	ids := registerRoots(t, l, l.movies, fourKLib)
	other, err := library.AddMovie(context.Background(), l.st, metadata.Movie{Name: "Other Film", Year: 2000, IDs: map[string]string{"tmdb": "910000"}},
		library.AddOptions{LibraryFolderID: ids[l.movies], Adopt: &library.Adoption{FolderName: "The Matrix (1999)"}}, nil, time.Now(), domain.System)
	if err != nil {
		t.Fatal(err)
	}
	mv := l.move(t, Choices{})
	got := lineOf(t, mv.Report, "title:movie:tmdb:603")
	if got.Result != ResFailed || !strings.Contains(got.Detail, "belongs to another movie") {
		t.Fatalf("title line %+v", got)
	}
	if n := l.count(t, "SELECT count(*) FROM versions WHERE title_id = ?", other); n != 1 {
		t.Fatalf("the other movie got %d Versions", n)
	}
	if u := l.undo(t, mv.ID); u.State != "undone" {
		t.Fatalf("undo %+v", u)
	}
	if n := l.count(t, "SELECT count(*) FROM titles WHERE id = ? AND state != 'removed'", other); n != 1 {
		t.Fatal("undo removed the other movie")
	}
}

// The 4K copy links the file the 4K app recorded, not a bigger alternate
// cut beside it.
func TestFourKLinksTheRecordedFile(t *testing.T) {
	l, fourKFile, _ := fourKLab(t, false)
	alt := filepath.Join(filepath.Dir(fourKFile), "Alternate theatrical cut.mkv")
	if err := os.WriteFile(alt, []byte(strings.Repeat("a much bigger alternate cut ", 50)), 0o644); err != nil {
		t.Fatal(err)
	}
	if mv := l.move(t, Choices{}); mv.State != "done" {
		t.Fatalf("move %+v", mv)
	}
	var rel string
	if err := l.st.Read().QueryRow("SELECT f.relative_path FROM files f JOIN versions v ON v.id = f.version_id WHERE v.four_k = 1").Scan(&rel); err != nil {
		t.Fatal(err)
	}
	if rel != "The Matrix (1999)/The.Matrix.1999.2160p.mkv" {
		t.Fatalf("4K file %s", rel)
	}
}

// Import, undo, import again with both roots already on the server: the
// undone 4K Version no longer holds its folder, so it comes over again.
func TestFourKImportUndoImportAgain(t *testing.T) {
	l, _, fourKLib := fourKLab(t, false)
	registerRoots(t, l, l.movies, fourKLib)
	first := l.move(t, Choices{})
	if first.State != "done" {
		t.Fatalf("move %+v", first)
	}
	if u := l.undo(t, first.ID); u.State != "undone" {
		t.Fatalf("undo %+v", u)
	}
	again := l.move(t, Choices{})
	if again.State != "done" {
		t.Fatalf("second move %+v", again)
	}
	t.Logf("second line %+v", lineOf(t, again.Report, "title:movie:tmdb:603"))
	if n := l.count(t, "SELECT count(*) FROM versions v JOIN titles t ON t.id = v.title_id WHERE v.four_k = 1 AND t.state != 'removed'"); n != 1 {
		t.Fatalf("%d active 4K Versions after import, undo, import", n)
	}
}

// TestMoveFourKCopySharingTheFolder: both Radarrs keep the movie in one
// folder, the 2160p file bigger than the 1080p one. The dry run and the
// real run agree that the 4K copy is not brought, the main Version keeps
// the file the 1080p app recorded instead of the bigger 2160p one, and no
// file changes on disk. That holds when either app recorded no quality for
// its file, and with an unrecorded older cut in the folder, bigger than the
// 1080p file. The 1080p app did not keep the movie up to date and the 4K
// app did, but with no 4K Version to upgrade the title stays as the 1080p
// app had it.
func TestMoveFourKCopySharingTheFolder(t *testing.T) {
	for _, c := range []struct {
		fourKFirst            bool
		quality, fourKQuality string
		oldCut                bool
	}{
		{false, "Bluray-1080p", "Remux-2160p", false}, {true, "Bluray-1080p", "Remux-2160p", false},
		{false, "", "Remux-2160p", true}, {true, "", "Remux-2160p", true},
		{false, "", "", false}, {true, "", "", true},
	} {
		fourKFirst := c.fourKFirst
		name := "1080p read first"
		if fourKFirst {
			name = "4K read first"
		}
		if c.quality == "" {
			name += ", no 1080p quality"
		}
		if c.fourKQuality == "" {
			name += ", no 4K quality"
		}
		if c.oldCut {
			name += ", an older cut"
		}
		t.Run(name, func(t *testing.T) {
			l := newMoveLabWith(t, func(radarr, _, _ *Snapshot) {
				radarr.QualityProfiles = []Profile{{ID: 2, Name: "HD-1080p", Cutoff: "Bluray-1080p"}}
				radarr.Titles[0].Files = []TitleFile{{Name: "The.Matrix.1999.1080p.mkv", Quality: c.quality}}
			})
			fourKFile := filepath.Join(filepath.Dir(l.file), "The.Matrix.1999.2160p.mkv")
			if err := os.WriteFile(fourKFile, []byte("the 4K copy, a good deal bigger than the 1080p one"), 0o644); err != nil {
				t.Fatal(err)
			}
			if c.oldCut {
				if err := os.WriteFile(filepath.Join(filepath.Dir(l.file), "The.Matrix.1999.Old.Cut.mkv"), []byte("an older cut, bigger than 1080p"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			uhd := &Snapshot{Kind: Radarr, Via: ViaAPI,
				Folders:         []Folder{{Path: "/movies"}},
				QualityProfiles: []Profile{{ID: 2, Name: "Ultra-HD", Cutoff: "Remux-2160p"}},
				Titles: []Title{{SourceID: 1, Kind: "movie", Name: "The Matrix", Year: 1999, IDs: map[string]string{"tmdb": "603"},
					Path: "/movies/The Matrix (1999)", Monitored: true, ProfileID: 2,
					Files: []TitleFile{{Name: "The.Matrix.1999.2160p.mkv", Quality: c.fourKQuality}}}}}
			uhd.finish()
			saved, err := Save(context.Background(), l.st, 1, uhd)
			if err != nil {
				t.Fatal(err)
			}
			if fourKFirst {
				l.snaps = append([]int64{saved.ID}, l.snaps...)
			} else {
				l.snaps = append(l.snaps, saved.ID)
			}
			mainBefore, fourKBefore := fileHash(t, l.file), fileHash(t, fourKFile)
			by := domain.Actor{Kind: "user", ID: 1}
			dry := l.mv.Run(context.Background(), 0, l.plan(t), true, by, nil)
			if dl := lineOf(t, dry, "title:movie:tmdb:603"); dl.Result != ResAdopt || !strings.Contains(dl.Detail, "its 4K copy is not placed: it shares the folder") {
				t.Fatalf("dry run %+v", dl)
			}
			mv := l.move(t, Choices{})
			if mv.State != "done" || mv.Report == nil {
				t.Fatalf("move %+v", mv)
			}
			if got := lineOf(t, mv.Report, "title:movie:tmdb:603"); got.Result != ResAdopted || !strings.Contains(got.Detail, "its 4K copy was not brought: it shares the folder") {
				t.Fatalf("title line %+v", got)
			}
			if n := l.count(t, "SELECT count(*) FROM versions v JOIN titles t ON t.id = v.title_id WHERE t.state != 'removed'"); n != 1 {
				t.Fatalf("%d Versions, want the main one only", n)
			}
			var rel, quality string
			if err := l.st.Read().QueryRow(`SELECT f.relative_path, COALESCE(json_extract(f.media_info, '$.recorded_quality'), '')
				FROM files f JOIN versions v ON v.id = f.version_id WHERE v.is_main = 1 AND f.state = 'present'`).Scan(&rel, &quality); err != nil {
				t.Fatal(err)
			}
			if rel != "The Matrix (1999)/The.Matrix.1999.1080p.mkv" || quality != c.quality {
				t.Fatalf("main file %q (%q), want the 1080p app's file", rel, quality)
			}
			if n := l.count(t, "SELECT count(*) FROM titles WHERE state != 'removed' AND keep_up_to_date = 1"); n != 0 {
				t.Fatal("the title is kept up to date with no 4K Version to upgrade")
			}
			if fileHash(t, l.file) != mainBefore || fileHash(t, fourKFile) != fourKBefore {
				t.Fatal("the move changed a media file's bytes")
			}
		})
	}
}

// TestMoveFourKCopyWhereTheMainCopyLands: the 1080p app's Library folder
// is not on this server, so the main copy comes over in the 4K Library
// folder, in the folder of the same name: the 4K copy's own, whether that
// folder holds the 4K file or is not there yet. The dry run and the real
// run agree that the 4K copy is not brought, in either read order, and the
// title stays as the 1080p app had it.
func TestMoveFourKCopyWhereTheMainCopyLands(t *testing.T) {
	for _, c := range []struct{ fourKFirst, fourKOnDisk bool }{{false, true}, {true, true}, {false, false}, {true, false}} {
		fourKFirst := c.fourKFirst
		name := "1080p read first"
		if fourKFirst {
			name = "4K read first"
		}
		if !c.fourKOnDisk {
			name += ", no 4K folder yet"
		}
		t.Run(name, func(t *testing.T) {
			l, fourKFile, _ := fourKLab(t, fourKFirst)
			// The 1080p app's folder is somewhere this server cannot see.
			if err := os.RemoveAll(l.movies); err != nil {
				t.Fatal(err)
			}
			if !c.fourKOnDisk {
				if err := os.RemoveAll(filepath.Dir(fourKFile)); err != nil {
					t.Fatal(err)
				}
			}
			var fourKBefore [32]byte
			if c.fourKOnDisk {
				fourKBefore = fileHash(t, fourKFile)
			}
			by := domain.Actor{Kind: "user", ID: 1}
			dry := l.mv.Run(context.Background(), 0, l.plan(t), true, by, nil)
			if dl := lineOf(t, dry, "title:movie:tmdb:603"); !strings.Contains(dl.Detail, "its 4K copy is not placed: the main copy comes over in its folder") {
				t.Fatalf("dry run %+v", dl)
			}
			mv := l.move(t, Choices{})
			if mv.State != "done" || mv.Report == nil {
				t.Fatalf("move %+v", mv)
			}
			got := lineOf(t, mv.Report, "title:movie:tmdb:603")
			if !strings.Contains(got.Detail, "its 4K copy was not brought: the main copy comes over in its folder") {
				t.Fatalf("title line %+v", got)
			}
			if n := l.count(t, "SELECT count(*) FROM versions v JOIN titles t ON t.id = v.title_id WHERE t.state != 'removed'"); n != 1 {
				t.Fatalf("%d Versions, want the main one only (%+v)", n, got)
			}
			if n := l.count(t, "SELECT count(*) FROM titles WHERE state != 'removed' AND keep_up_to_date = 1"); n != 0 {
				t.Fatal("the title is kept up to date with no 4K Version to upgrade")
			}
			if c.fourKOnDisk && fileHash(t, fourKFile) != fourKBefore {
				t.Fatal("the move changed a media file's bytes")
			}
		})
	}
}

// TestMoveAgainKeepsEveryVersionsQualityForOneApp: a movie from one Radarr
// (no 4K merge) whose title was given a second Version holding a file of
// the same name gets the quality the app recorded on both files when the
// move runs again, as it did before 4K merges.
func TestMoveAgainKeepsEveryVersionsQualityForOneApp(t *testing.T) {
	l := newMoveLabWith(t, func(radarr, _, _ *Snapshot) {
		radarr.Titles[0].Files = []TitleFile{{Name: "The.Matrix.1999.1080p.mkv", Quality: "Bluray-1080p"}}
	})
	if mv := l.move(t, Choices{}); mv.State != "done" {
		t.Fatalf("move %+v", mv)
	}
	ctx := context.Background()
	var titleID, lfID int64
	if err := l.st.Read().QueryRow(`SELECT v.title_id, v.library_folder_id FROM versions v JOIN titles t ON t.id = v.title_id
		WHERE t.state != 'removed' AND v.is_main = 1`).Scan(&titleID, &lfID); err != nil {
		t.Fatal(err)
	}
	copyDir := filepath.Join(l.movies, "The Matrix (1999) [Copy]")
	if err := os.MkdirAll(copyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(copyDir, "The.Matrix.1999.1080p.mkv"), []byte("a second copy"), 0o644); err != nil {
		t.Fatal(err)
	}
	by := domain.Actor{Kind: "user", ID: 1}
	v, err := library.AddVersion(ctx, l.st, titleID, library.VersionInput{Label: "Copy", LibraryFolderID: lfID, FolderName: "The Matrix (1999) [Copy]", ExistingFolder: true}, nil, time.Now(), by)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.mv.Library.AdoptMovieCopy(ctx, titleID, v.ID, library.CopyAdoption{RelativePath: "The Matrix (1999) [Copy]/The.Matrix.1999.1080p.mkv"}, by); err != nil {
		t.Fatal(err)
	}
	if again := l.move(t, Choices{}); again.State != "done" {
		t.Fatalf("second move %+v", again)
	}
	var q string
	if err := l.st.Read().QueryRow(`SELECT COALESCE(json_extract(media_info, '$.recorded_quality'), '') FROM files
		WHERE version_id = ? AND state = 'present'`, v.ID).Scan(&q); err != nil {
		t.Fatal(err)
	}
	if q != "Bluray-1080p" {
		t.Fatalf("the second Version's file has recorded quality %q, want the app's Bluray-1080p", q)
	}
}

// TestMoveFourKCopyAndAnotherMoviesFallback: another movie whose Library
// folder this server cannot see comes over in the 4K Library folder, in a
// folder named like the merged movie's 4K copy, which holds the 4K file.
// It comes over wanted there; when it comes first in the import its folder
// leaves the 4K copy none, and the dry run says what the real run does in
// both orders. The other movie is in a third Radarr, read first or last.
func TestMoveFourKCopyAndAnotherMoviesFallback(t *testing.T) {
	for _, otherFirst := range []bool{true, false} {
		name := "the other movie first"
		if !otherFirst {
			name = "the merged movie first"
		}
		t.Run(name, func(t *testing.T) {
			l, _, _ := fourKLab(t, true)
			oth := &Snapshot{Kind: Radarr, Via: ViaAPI,
				Folders:         []Folder{{Path: "/gone"}},
				QualityProfiles: []Profile{{ID: 2, Name: "Ultra-HD", Cutoff: "Remux-2160p"}},
				Titles: []Title{{SourceID: 1, Kind: "movie", Name: "Other", Year: 2000, IDs: map[string]string{"tmdb": "999"},
					Path: "/gone/The Matrix (1999)", Monitored: true, ProfileID: 2}}}
			oth.finish()
			saved, err := Save(context.Background(), l.st, 1, oth)
			if err != nil {
				t.Fatal(err)
			}
			if otherFirst {
				l.snaps = append([]int64{saved.ID}, l.snaps...)
			} else {
				l.snaps = append(l.snaps, saved.ID)
			}
			l.mv.Library.Metadata = &metadata.Static{Movies: []metadata.Movie{
				{Name: "The Matrix", Year: 1999, IDs: map[string]string{"tmdb": "603", "imdb": "tt0133093"}},
				{Name: "Other", Year: 2000, IDs: map[string]string{"tmdb": "999"}}}}
			by := domain.Actor{Kind: "user", ID: 1}
			dry := l.mv.Run(context.Background(), 0, l.plan(t), true, by, nil)
			dryMerged, dryOther := lineOf(t, dry, "title:movie:tmdb:603"), lineOf(t, dry, "title:movie:tmdb:999")
			mv := l.move(t, Choices{})
			if mv.State != "done" || mv.Report == nil {
				t.Fatalf("move %+v", mv)
			}
			merged, otherLine := lineOf(t, mv.Report, "title:movie:tmdb:603"), lineOf(t, mv.Report, "title:movie:tmdb:999")
			if dryOther.Result != ResCreate || otherLine.Result != ResCreated {
				t.Fatalf("other movie: dry %+v, real %+v", dryOther, otherLine)
			}
			fourKVersions := l.count(t, "SELECT count(*) FROM versions v JOIN titles t ON t.id = v.title_id WHERE t.state != 'removed' AND v.four_k = 1 AND v.is_main = 0")
			if otherFirst {
				if !strings.Contains(dryMerged.Detail, "its 4K copy will not be brought: the folder") || !strings.Contains(merged.Detail, "its 4K copy was not brought") || fourKVersions != 0 {
					t.Fatalf("merged movie: dry %+v, real %+v; %d 4K Versions", dryMerged, merged, fourKVersions)
				}
				return
			}
			if !strings.Contains(dryMerged.Detail, "its 4K copy becomes a 4K Version") || !strings.Contains(merged.Detail, "its 4K copy came over as a 4K Version") || fourKVersions != 1 {
				t.Fatalf("merged movie: dry %+v, real %+v; %d 4K Versions", dryMerged, merged, fourKVersions)
			}
		})
	}
}

// TestMoveFourKCopyIntoAMissingLibraryFolder: the 4K Library folder is in
// the Library already but gone from disk. The dry run and the real run
// agree that the 4K copy is not brought, and the title stays as the 1080p
// app had it (not kept up to date), not kept up to date for a 4K copy that
// never came. When the folder is back, a second run leaves the title as it
// is and says to add the 4K copy on the title page, so each run's undo
// takes back only what that run made; undoing it changes nothing.
func TestMoveFourKCopyIntoAMissingLibraryFolder(t *testing.T) {
	l, fourKFile, fourKLib := fourKLab(t, false)
	if _, err := library.AddFolder(context.Background(), l.st, library.FolderInput{Path: fourKLib, Kind: "movie", Label: "Movies 4K"}, nil, domain.System); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(fourKLib); err != nil {
		t.Fatal(err)
	}
	by := domain.Actor{Kind: "user", ID: 1}
	dry := l.mv.Run(context.Background(), 0, l.plan(t), true, by, nil)
	if dl := lineOf(t, dry, "title:movie:tmdb:603"); !strings.Contains(dl.Detail, "its 4K copy will not be brought") {
		t.Fatalf("dry run %+v", dl)
	}
	mv := l.move(t, Choices{})
	if mv.State != "done" || mv.Report == nil {
		t.Fatalf("move %+v", mv)
	}
	if got := lineOf(t, mv.Report, "title:movie:tmdb:603"); !strings.Contains(got.Detail, "its 4K copy was not brought") {
		t.Fatalf("title line %+v", got)
	}
	if n := l.count(t, "SELECT count(*) FROM versions v JOIN titles t ON t.id = v.title_id WHERE t.state != 'removed' AND v.four_k = 1 AND v.is_main = 0"); n != 0 {
		t.Fatalf("%d 4K Versions", n)
	}
	if n := l.count(t, "SELECT count(*) FROM titles WHERE state != 'removed' AND keep_up_to_date = 1"); n != 0 {
		t.Fatal("the title is kept up to date for a 4K copy that never came")
	}
	if n := l.count(t, "SELECT count(*) FROM versions v JOIN titles t ON t.id = v.title_id WHERE t.state != 'removed' AND v.is_main = 1 AND v.keep_up_to_date = 0"); n != 0 {
		t.Fatal("the main copy was switched off for a 4K copy that never came")
	}
	// The folder comes back and the import runs again.
	if err := os.MkdirAll(filepath.Dir(fourKFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fourKFile, []byte("the 4K copy, bigger"), 0o644); err != nil {
		t.Fatal(err)
	}
	dry = l.mv.Run(context.Background(), 0, l.plan(t), true, by, nil)
	if dl := lineOf(t, dry, "title:movie:tmdb:603"); dl.Result != ResAlready || !strings.Contains(dl.Detail, "imported before; add its 4K copy on the title page") {
		t.Fatalf("second dry run %+v", dl)
	}
	again := l.move(t, Choices{})
	if again.State != "done" || again.Report == nil {
		t.Fatalf("second move %+v", again)
	}
	if got := lineOf(t, again.Report, "title:movie:tmdb:603"); got.Result != ResAlready || !strings.Contains(got.Detail, "imported before; add its 4K copy on the title page") {
		t.Fatalf("second title line %+v", got)
	}
	stays := func(when string) {
		t.Helper()
		if n := l.count(t, "SELECT count(*) FROM versions v JOIN titles t ON t.id = v.title_id WHERE t.state != 'removed' AND v.four_k = 1 AND v.is_main = 0"); n != 0 {
			t.Fatalf("%s: %d 4K Versions", when, n)
		}
		if n := l.count(t, "SELECT count(*) FROM titles WHERE state != 'removed' AND keep_up_to_date = 0"); n != 1 {
			t.Fatalf("%s: the title's keep up to date changed", when)
		}
	}
	stays("after the second run")
	if u := l.undo(t, again.ID); u.State != "undone" {
		t.Fatalf("undo of the second run %+v", u)
	}
	stays("after undoing the second run")
	if _, err := os.Stat(fourKFile); err != nil {
		t.Fatal("undo touched the 4K file:", err)
	}
}

// TestMoveFourKCopyIntoAReadOnlyLibraryFolder: the 4K Library folder this
// import adds cannot be written, so adding a Version there is refused; the
// dry run says the same instead of promising the 4K copy.
func TestMoveFourKCopyIntoAReadOnlyLibraryFolder(t *testing.T) {
	l, _, fourKLib := fourKLab(t, false)
	if err := os.Chmod(fourKLib, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(fourKLib, 0o755) })
	if f, err := os.CreateTemp(fourKLib, "probe"); err == nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		t.Skip("this system lets the test write to a read-only folder")
	}
	by := domain.Actor{Kind: "user", ID: 1}
	dry := l.mv.Run(context.Background(), 0, l.plan(t), true, by, nil)
	if dl := lineOf(t, dry, "title:movie:tmdb:603"); !strings.Contains(dl.Detail, "its 4K copy will not be brought: the Library folder is not usable") {
		t.Fatalf("dry run %+v", dl)
	}
	mv := l.move(t, Choices{})
	if mv.State != "done" || mv.Report == nil {
		t.Fatalf("move %+v", mv)
	}
	if got := lineOf(t, mv.Report, "title:movie:tmdb:603"); !strings.Contains(got.Detail, "its 4K copy was not brought") {
		t.Fatalf("title line %+v", got)
	}
}

// TestMoveFourKCopyInAFolderLeadingElsewhere: the 4K movie folder is a link
// to a folder outside its Library folder, so adopting its file is refused
// and the 4K Version is taken back; the dry run says the same.
func TestMoveFourKCopyInAFolderLeadingElsewhere(t *testing.T) {
	l, fourKFile, _ := fourKLab(t, false)
	elsewhere := filepath.Join(t.TempDir(), "The Matrix (1999)")
	if err := os.Rename(filepath.Dir(fourKFile), elsewhere); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, filepath.Dir(fourKFile)); err != nil {
		t.Skip("symlinks not available:", err)
	}
	by := domain.Actor{Kind: "user", ID: 1}
	dry := l.mv.Run(context.Background(), 0, l.plan(t), true, by, nil)
	if dl := lineOf(t, dry, "title:movie:tmdb:603"); !strings.Contains(dl.Detail, "its 4K copy will not be brought: its file") {
		t.Fatalf("dry run %+v", dl)
	}
	mv := l.move(t, Choices{})
	if mv.State != "done" || mv.Report == nil {
		t.Fatalf("move %+v", mv)
	}
	if got := lineOf(t, mv.Report, "title:movie:tmdb:603"); !strings.Contains(got.Detail, "its 4K copy was not brought") {
		t.Fatalf("title line %+v", got)
	}
	if n := l.count(t, "SELECT count(*) FROM versions v JOIN titles t ON t.id = v.title_id WHERE t.state != 'removed' AND v.four_k = 1 AND v.is_main = 0"); n != 0 {
		t.Fatalf("%d 4K Versions", n)
	}
	if _, err := os.Stat(filepath.Join(elsewhere, filepath.Base(fourKFile))); err != nil {
		t.Fatal("the 4K file is gone:", err)
	}
}

// TestMoveFourKCopyIntoAnOverlappingLibraryFolder: the 4K Library folder
// lies inside a Library folder already kept, so adding it is refused; the
// dry run says the 4K copy will not be brought instead of promising it.
func TestMoveFourKCopyIntoAnOverlappingLibraryFolder(t *testing.T) {
	l, _, fourKLib := fourKLab(t, false)
	if _, err := library.AddFolder(context.Background(), l.st, library.FolderInput{Path: filepath.Dir(fourKLib), Kind: "movie", Label: "Everything"}, nil, domain.System); err != nil {
		t.Fatal(err)
	}
	by := domain.Actor{Kind: "user", ID: 1}
	dry := l.mv.Run(context.Background(), 0, l.plan(t), true, by, nil)
	if dl := lineOf(t, dry, "title:movie:tmdb:603"); !strings.Contains(dl.Detail, "overlaps another Library folder, so it cannot be added") {
		t.Fatalf("dry run %+v", dl)
	}
	mv := l.move(t, Choices{})
	if mv.Report == nil {
		t.Fatalf("move %+v", mv)
	}
	if got := lineOf(t, mv.Report, "title:movie:tmdb:603"); !strings.Contains(got.Detail, "its 4K copy was not brought") {
		t.Fatalf("title line %+v", got)
	}
}

// extraReadAt saves s as the lab's read at index at, with a path rule
// from its one folder /media to the folder to.
func extraReadAt(t *testing.T, l *moveLab, s *Snapshot, at int, to string) {
	t.Helper()
	s.Via, s.Folders = ViaAPI, []Folder{{Path: "/media"}}
	s.finish()
	saved, err := Save(context.Background(), l.st, 1, s)
	if err != nil {
		t.Fatal(err)
	}
	l.snaps = append(append(append([]int64{}, l.snaps[:at]...), saved.ID), l.snaps[at:]...)
	l.rules = append(l.rules, PathRule{From: "/media", To: filepath.ToSlash(to)})
}

// readBeforeFourK saves s as a read that comes just before the lab's 4K
// read, its folder /media holding the 4K Library folder.
func readBeforeFourK(t *testing.T, l *moveLab, s *Snapshot, fourKLib string) {
	t.Helper()
	extraReadAt(t, l, s, len(l.snaps)-1, filepath.Dir(fourKLib))
}

// TestMoveFourKCopyAfterAnUnsafeFolder: a show Library folder read first
// holds every other folder of the import but also HolyArrs's own folder,
// so the run refuses it and adds the movie folders after it; the dry run
// promises the 4K copy the real run brings.
func TestMoveFourKCopyAfterAnUnsafeFolder(t *testing.T) {
	l, _, fourKLib := fourKLab(t, false)
	around := filepath.Dir(l.st.Dir())
	if !library.FoldersOverlap(around, fourKLib) || !library.FoldersOverlap(around, l.movies) {
		t.Fatalf("%s does not hold the lab's Library folders", around)
	}
	extraReadAt(t, l, &Snapshot{Kind: Sonarr, Version: "4.0.16"}, 0, around)
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

// TestMoveFourKCopyIntoAnUnsafeLibraryFolder: the 4K Library folder lies
// inside HolyArrs's own folder, so adding it is refused; the dry run says
// the 4K copy will not be brought instead of promising it.
func TestMoveFourKCopyIntoAnUnsafeLibraryFolder(t *testing.T) {
	l, _, _ := fourKLab(t, false)
	inside := filepath.Join(l.st.Dir(), "4k")
	file := filepath.Join(inside, "The Matrix (1999)", "The.Matrix.1999.2160p.mkv")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("the 4K copy, bigger"), 0o644); err != nil {
		t.Fatal(err)
	}
	l.rules[len(l.rules)-1].To = filepath.ToSlash(inside)
	by := domain.Actor{Kind: "user", ID: 1}
	dry := l.mv.Run(context.Background(), 0, l.plan(t), true, by, nil)
	if dl := lineOf(t, dry, "title:movie:tmdb:603"); !strings.Contains(dl.Detail, "cannot be added: "+library.ErrFolderPath.Error()) {
		t.Fatalf("dry run %+v", dl)
	}
	mv := l.move(t, Choices{})
	if mv.Report == nil {
		t.Fatalf("move %+v", mv)
	}
	if got := lineOf(t, mv.Report, "title:movie:tmdb:603"); !strings.Contains(got.Detail, "its 4K copy was not brought") {
		t.Fatalf("title line %+v", got)
	}
}

// TestMoveFourKCopyInsideAnEarlierShowFolder: a show Library folder this
// run adds first holds the 4K Library folder, so adding the 4K one is
// refused; the dry run says so, whatever kind the earlier folder is.
func TestMoveFourKCopyInsideAnEarlierShowFolder(t *testing.T) {
	l, _, fourKLib := fourKLab(t, false)
	readBeforeFourK(t, l, &Snapshot{Kind: Sonarr, Version: "4.0.16"}, fourKLib)
	by := domain.Actor{Kind: "user", ID: 1}
	dry := l.mv.Run(context.Background(), 0, l.plan(t), true, by, nil)
	if dl := lineOf(t, dry, "title:movie:tmdb:603"); !strings.Contains(dl.Detail, "overlaps another Library folder, so it cannot be added") {
		t.Fatalf("dry run %+v", dl)
	}
	mv := l.move(t, Choices{})
	if mv.Report == nil {
		t.Fatalf("move %+v", mv)
	}
	if got := lineOf(t, mv.Report, "title:movie:tmdb:603"); !strings.Contains(got.Detail, "its 4K copy was not brought") {
		t.Fatalf("title line %+v", got)
	}
}

// TestMoveFourKCopyAfterARefusedFolder: a movie Library folder planned
// before the 4K one holds it but is refused itself (it overlaps a show
// folder already kept), so it takes no path and the 4K Library folder is
// added; the dry run promises the 4K copy the real run brings.
func TestMoveFourKCopyAfterARefusedFolder(t *testing.T) {
	l, _, fourKLib := fourKLab(t, false)
	shows := filepath.Join(filepath.Dir(fourKLib), "shows")
	if err := os.MkdirAll(shows, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := library.AddFolder(context.Background(), l.st, library.FolderInput{Path: shows, Kind: "show", Label: "Shows"}, nil, domain.System); err != nil {
		t.Fatal(err)
	}
	readBeforeFourK(t, l, &Snapshot{Kind: Radarr, Version: "6.4.4"}, fourKLib)
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

// TestMoveFourKCopyIntoALookOnlyLibraryFolder: a look-only 4K Library
// folder keeps its 4K Version switched off, so the title is not switched to
// kept up to date for it, and the main copy stays on, as the 1080p app had
// the title.
func TestMoveFourKCopyIntoALookOnlyLibraryFolder(t *testing.T) {
	l, _, fourKLib := fourKLab(t, false)
	if _, err := library.AddFolder(context.Background(), l.st, library.FolderInput{Path: fourKLib, Kind: "movie", Label: "Movies 4K", LookOnly: true}, nil, domain.System); err != nil {
		t.Fatal(err)
	}
	mv := l.move(t, Choices{})
	if mv.Report == nil {
		t.Fatalf("move %+v", mv)
	}
	if n := l.count(t, "SELECT count(*) FROM versions v JOIN titles t ON t.id = v.title_id WHERE t.state != 'removed' AND v.four_k = 1 AND v.is_main = 0 AND v.keep_up_to_date = 1"); n != 0 {
		t.Fatalf("%d 4K Versions kept up to date in a look-only folder", n)
	}
	if n := l.count(t, "SELECT count(*) FROM titles WHERE state != 'removed' AND keep_up_to_date = 1"); n != 0 {
		t.Fatal("the title is kept up to date for a 4K Version that is switched off")
	}
	if n := l.count(t, "SELECT count(*) FROM versions v JOIN titles t ON t.id = v.title_id WHERE t.state != 'removed' AND v.is_main = 1 AND v.keep_up_to_date = 0"); n != 0 {
		t.Fatal("the main copy was switched off for a 4K Version that is switched off")
	}
}
