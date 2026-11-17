package downloads

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A finished row whose files are in or around a Library folder says so, so
// the Remove confirm never promises to delete what the server keeps.
func TestDownloadsViewFinishedRowSaysFilesStayInLibrary(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		lib  string
		stay bool
	}{{"/moved", true}, {"/moved/complete/Salt.Choir.mkv/x", true}, {"/media/movies", false}} {
		x := seed41(t, mediafiles.SlotInLibrary, "finished")
		exec41(t, x.s, "UPDATE library_folders SET path = ?", c.lib)
		pointQB(t, x, qbList(t, `[{"hash":"abcdabcdabcdabcdabcdabcdabcdabcdabcdabcd","name":"Salt.Choir","category":"holyarrs","size":1000,"progress":1,"state":"uploading","save_path":"/moved/complete","content_path":"/moved/complete/Salt.Choir.mkv","added_on":1700000000}]`).URL)
		feedStore(t, x.dl)
		v := view(t, x.srv.URL+"/api/v1/downloads/view")
		if len(v.Downloads) == 0 {
			t.Fatalf("%s: no own row", c.lib)
		}
		if got := v.Downloads[0].FilesStay; got != c.stay {
			t.Errorf("library %s: files_stay %v, want %v", c.lib, got, c.stay)
		}
	}
}
