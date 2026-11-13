package importer

import (
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// The no-match reason says what happened and what to do, with no "import plan".
func TestNoMatchReasonSaysWhatToDo(t *testing.T) {
	im := &Importer{SavePath: "/data"}
	files := []domain.SeenFile{{Name: "x.mkv"}}
	for got, want := range map[string]string{
		im.nothingPlaced(domain.Downloader{}, files):         "no file in the download matched this movie; match the files by hand",
		im.nothingMatched(domain.Downloader{}, files, false): "no file in the download matched an episode; match the files by hand",
		im.nothingMatched(domain.Downloader{}, files, true):  "no file in the download matched a track; match the files by hand",
	} {
		if got != want || strings.Contains(got, "import plan") {
			t.Fatalf("got %q want %q", got, want)
		}
	}
}
