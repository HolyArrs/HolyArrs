package downloads

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// importRow records one Import of the rig's download in a state.
func importRow(t *testing.T, x *dl41, state string) {
	t.Helper()
	exec41(t, x.s, `INSERT INTO imports(created_at, created_by, updated_at, updated_by, download_id, from_path, kind, mode, state)
		VALUES ('t', '{}', 't', '{}', ?, '/data/downloads', 'movie', 'auto', ?)`, x.dlID, state)
}

func ownRow(t *testing.T, x *dl41) DownloadRow {
	t.Helper()
	for _, r := range view(t, x.srv.URL+"/api/v1/downloads/view").Downloads {
		if r.ID == x.dlID {
			return r
		}
	}
	t.Fatal("the download is not on the page")
	return DownloadRow{}
}

// A refused import never reads as in the library, whether the Slot kept the
// copy it had or went looking for another one.
func TestDownloadsViewRefusedImportIsNotDone(t *testing.T) {
	t.Parallel()
	for slot, text := range map[mediafiles.SlotState]string{
		mediafiles.SlotInLibrary: "The files were not what the name said; the copy already in the library stays.",
		mediafiles.SlotSearching: "The files were not what the name said; HolyArrs looks for another copy.",
		mediafiles.SlotWanted:    "The files were not what the name said; HolyArrs looks for another copy.",
	} {
		x := seed41(t, slot, "finished")
		importRow(t, x, "rejected")
		if r := ownRow(t, x); r.Phase != "import_refused" || r.Label != "Import refused" || r.Text != text {
			t.Fatalf("slot %s: phase %q label %q text %q", slot, r.Phase, r.Label, r.Text)
		}
	}
}

// An import done after a refused one (a person let it in) is in the library.
func TestDownloadsViewImportDoneAfterARefusalIsDone(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotInLibrary, "finished")
	importRow(t, x, "rejected")
	importRow(t, x, "done")
	if r := ownRow(t, x); r.Phase != "done" || r.Text != "In the library." {
		t.Fatalf("phase %q text %q", r.Phase, r.Text)
	}
}

// A download whose Release is gone (its Source was deleted) is named
// by what the Downloader held, else by its Title, never by a bare hash.
func TestDownloadsViewNamesADownloadWhoseReleaseIsGone(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotRemoved, "finished")
	exec41(t, x.s, "UPDATE downloads SET release_id = NULL")
	if r := ownRow(t, x); r.Name != "Salt Choir" {
		t.Fatalf("no files seen: name %q", r.Name)
	}
	exec41(t, x.s, `UPDATE downloads SET files_seen = '[{"name":"Salt.Choir.2020.1080p.WEB.H264-HOLYARRS.mkv","size":10}]'`)
	if r := ownRow(t, x); r.Name != "Salt.Choir.2020.1080p.WEB.H264-HOLYARRS" {
		t.Fatalf("one file: name %q", r.Name)
	}
	exec41(t, x.s, `UPDATE downloads SET files_seen = '[{"name":"Salt.Choir.2020.1080p-GRP/salt.mkv","size":10},{"name":"Salt.Choir.2020.1080p-GRP/salt.nfo","size":1}]'`)
	if r := ownRow(t, x); r.Name != "Salt.Choir.2020.1080p-GRP" {
		t.Fatalf("a folder: name %q", r.Name)
	}
}

func TestSeenName(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"":                        "",
		"a.mkv":                   "a",
		"Show.S01/ep1.mkv":        "Show.S01",
		`Show.S01\ep1.mkv`:        "Show.S01",
		"/Show.S01/ep1.mkv":       "Show.S01",
		"..":                      "",
		"noext":                   "noext",
		"Movie.2020.1080p-GRP.x":  "Movie.2020.1080p-GRP",
		"Movie.2020.WEB.H264-GRP": "Movie.2020.WEB.H264-GRP",
		"Salt.Choir.2020":         "Salt.Choir.2020",
	} {
		var files []domain.SeenFile
		if in != "" {
			files = []domain.SeenFile{{Name: in}}
		}
		if got := seenName(files); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
	if got := seenName([]domain.SeenFile{{Name: "sample.mkv"}, {Name: "movie.mkv"}}); got != "" {
		t.Errorf("loose files with no folder: %q", got)
	}
}
