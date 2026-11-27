package importer

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

// What the report describes: a fansub release names the quality, the file it
// ships does not.
const (
	issue770Release = "[EMBER] Dandadan S01E01 [1080p] [Dual Audio HEVC WEBRip DDP]"
	issue770File    = "[EMBER] Dandadan - 01.mkv"
	issue770Folder  = "Dandadan (2024)"
)

// dandadanWorld is showSetup for a download of the show the issue names, with
// rel as the grabbed release's name. Its files are named the fansub way, by
// running number, so their own names give no quality.
func dandadanWorld(t *testing.T, rel string, files ...string) showWorld {
	t.Helper()
	w := showSetup(t, []int{1}, files...)
	if err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		for _, q := range []string{
			"UPDATE titles SET name = 'Dandadan', sort_name = 'dandadan'",
			"UPDATE versions SET folder_name = 'Dandadan (2024)'",
			// A running number, as fansub files use.
			"UPDATE items SET numbers = json_set(numbers, '$.absolute', json_extract(numbers, '$.episode'))",
		} {
			if _, err := tx.Exec(q); err != nil {
				return err
			}
		}
		_, err := tx.Exec("UPDATE releases SET name = ? WHERE id = ?", rel, w.relID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return w
}

// The issue's own case: the release names the quality, the file does not, so
// the import keeps the release's quality and its name.
func TestShowImportKeepsTheGrabbedQualityAndReleaseName(t *testing.T) {
	w := dandadanWorld(t, issue770Release, issue770File)
	runImport(t, w)
	f := importedFile(t, w, 1)
	if f.Quality.Resolution != 1080 || f.Quality.Source != "web_rip" {
		t.Fatalf("quality %+v, want 1080p web_rip", f.Quality)
	}
	if f.ReleaseName == nil || *f.ReleaseName != issue770Release {
		t.Fatalf("release name %v, want %q", f.ReleaseName, issue770Release)
	}
	if want := issue770Folder + "/Season 1/Dandadan - S01E01 - The Mill 1080p WEB.mkv"; f.RelativePath != want {
		t.Fatalf("placed as %s, want %s", f.RelativePath, want)
	}
}

// When neither name gives a resolution the inspected header's is kept, so the
// file is not named SD.
func TestShowImportNamesTheFileFromTheHeaderResolution(t *testing.T) {
	w := dandadanWorld(t, "[EMBER] Dandadan S01 [Dual Audio]", issue770File)
	src := filepath.Join(w.dl, packName, issue770File)
	put(t, src, fixture(t, "intact.mkv", 0))
	if res, _ := InspectFacts(src)["resolution"].(string); res != "1080p" {
		t.Fatalf("the fixture's header reads as %q", res)
	}
	runImport(t, w)
	f := importedFile(t, w, 1)
	if f.Quality.Resolution != 1080 || f.MediaInfo["step"] != "1080p_web" {
		t.Fatalf("quality %+v, step %v", f.Quality, f.MediaInfo["step"])
	}
	// The name on disk and the stored step say the same quality.
	if want := issue770Folder + "/Season 1/Dandadan - S01E01 - The Mill 1080p WEB.mkv"; f.RelativePath != want {
		t.Fatalf("placed as %s, want %s", f.RelativePath, want)
	}
	if _, err := os.Stat(filepath.Join(w.lib, filepath.FromSlash(f.RelativePath))); err != nil {
		t.Fatalf("not on disk under that name: %v", err)
	}
}
