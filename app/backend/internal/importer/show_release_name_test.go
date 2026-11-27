package importer

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// releaseWorld is showSetup with the grabbed release named rel and each
// episode given its running number, as fansub files are named.
func releaseWorld(t *testing.T, rel string, eps []int, files ...string) showWorld {
	t.Helper()
	w := showSetup(t, eps, files...)
	if err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE releases SET name = ? WHERE id = ?", rel, w.relID); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE items SET numbers = json_set(numbers, '$.absolute', json_extract(numbers, '$.episode'))")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return w
}

func importedFile(t *testing.T, w showWorld, n int) mediafiles.File {
	t.Helper()
	sl := w.slot(t, n)
	if sl.FileID == nil {
		t.Fatalf("E%d was not placed: %+v", n, sl)
	}
	return w.file(t, *sl.FileID)
}

func runImport(t *testing.T, w showWorld) {
	t.Helper()
	imp, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System)
	if err != nil || imp.State != "done" {
		t.Fatalf("%v %+v", err, imp)
	}
}

func TestShowImportReadsQualityFromRelease(t *testing.T) {
	rel := "[EMBER] Copper Valley S01E01 [1080p] [Dual Audio HEVC WEBRip DDP]"
	w := releaseWorld(t, rel, []int{1}, "[EMBER] Copper Valley - 01.mkv")
	runImport(t, w)
	f := importedFile(t, w, 1)
	if f.Quality.Resolution != 1080 || f.Quality.Source != "web_rip" || f.Quality.Codec != "h265" {
		t.Fatalf("quality %+v", f.Quality)
	}
	if f.ReleaseName == nil || *f.ReleaseName != rel || f.Group == nil || *f.Group != "EMBER" {
		t.Fatalf("release name %v, group %v", f.ReleaseName, f.Group)
	}
	// The Library renames by the stored step, so it must not say SD TV either.
	if want := "Copper Valley (2025)/Season 1/Copper Valley - S01E01 - The Mill 1080p WEB.mkv"; f.RelativePath != want || f.MediaInfo["step"] != "1080p_web" {
		t.Fatalf("placed as %s, step %v; want %s", f.RelativePath, f.MediaInfo["step"], want)
	}
}

func TestShowImportPackFilesTakePackQuality(t *testing.T) {
	w := releaseWorld(t, "[EMBER] Copper Valley S01 [1080p] [Dual Audio HEVC WEBRip DDP]", []int{1, 2},
		"[EMBER] Copper Valley - 01.mkv", "[EMBER] Copper Valley - 02.mkv")
	runImport(t, w)
	names := []string{"", "The Mill", "Flood"}
	for n := 1; n <= 2; n++ {
		f := importedFile(t, w, n)
		own := "[EMBER] Copper Valley - 0" + string(rune('0'+n))
		if f.ReleaseName == nil || *f.ReleaseName != own {
			t.Fatalf("E%d release name %v, want the file's own %s", n, f.ReleaseName, own)
		}
		if f.Quality.Resolution != 1080 || f.Quality.Source != "web_rip" {
			t.Fatalf("E%d quality %+v", n, f.Quality)
		}
		if want := "Copper Valley (2025)/Season 1/Copper Valley - S01E0" + string(rune('0'+n)) + " - " + names[n] + " 1080p WEB.mkv"; f.RelativePath != want {
			t.Fatalf("E%d placed as %s, want %s", n, f.RelativePath, want)
		}
	}
}

func TestShowImportFileNameQualityWins(t *testing.T) {
	for _, c := range []struct {
		file, rel, source string
	}{
		// What the file name says for sure is kept over the release's word.
		{"Copper.Valley.S01E01.1080p.HDTV.x264-HOLYARRS.mkv", "Copper.Valley.S01E01.1080p.WEB.H264-HOLYARRS", "tv"},
		// A Sonarr-style name and release read as before.
		{ep(1), "Copper.Valley.S01E01.1080p.WEB.H264-HOLYARRS", "web_dl"},
	} {
		w := showSetup(t, []int{1}, c.file)
		if err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
			_, err := tx.Exec("UPDATE releases SET name = ? WHERE id = ?", c.rel, w.relID)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		runImport(t, w)
		f := importedFile(t, w, 1)
		if f.Quality.Resolution != 1080 || f.Quality.Source != c.source || f.Quality.Codec != "h264" {
			t.Fatalf("%s: quality %+v", c.file, f.Quality)
		}
		if f.ReleaseName == nil || *f.ReleaseName != c.rel {
			t.Fatalf("%s: release name %v", c.file, f.ReleaseName)
		}
	}
}

func TestShowImportResolutionFromHeader(t *testing.T) {
	w := releaseWorld(t, "[EMBER] Copper Valley S01 [Dual Audio]", []int{1}, "[EMBER] Copper Valley - 01.mkv")
	put(t, filepath.Join(w.dl, packName, "[EMBER] Copper Valley - 01.mkv"), fixture(t, "intact.mkv", 0))
	facts := InspectFacts(filepath.Join(w.dl, packName, "[EMBER] Copper Valley - 01.mkv"))
	res, _ := facts["resolution"].(string)
	if res != "1080p" {
		t.Fatalf("the fixture's header reads as %q", res)
	}
	runImport(t, w)
	f := importedFile(t, w, 1)
	// No name gives a resolution, so the header's is recorded and the
	// Library's renaming reads it from the step.
	if f.Quality.Resolution != 1080 || f.MediaInfo["step"] != "1080p_web" {
		t.Fatalf("quality %+v, step %v", f.Quality, f.MediaInfo["step"])
	}
}
