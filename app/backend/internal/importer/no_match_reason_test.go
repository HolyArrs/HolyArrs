package importer

import (
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// The no-match reason says what happened and what to do, with no "import plan".
func TestNoMatchReasonSaysWhatToDo(t *testing.T) {
	im := &Importer{SavePath: "/data"}
	files := []domain.SeenFile{{Name: "x.mkv"}}
	for got, want := range map[string]string{
		im.nothingPlaced(domain.Downloader{}, files, nil):         "no file in the download matched this movie; match the files by hand",
		im.nothingMatched(domain.Downloader{}, files, false, nil): "no file in the download matched an episode; match the files by hand",
		im.nothingMatched(domain.Downloader{}, files, true, nil):  "no file in the download matched a track; match the files by hand",
	} {
		if got != want || strings.Contains(got, "import plan") {
			t.Fatalf("got %q want %q", got, want)
		}
	}
}

// A refusal that is not the outside-the-folder one keeps the no-match lines.
func TestNoMatchReasonIgnoresOtherRefusals(t *testing.T) {
	im := &Importer{SavePath: "/data"}
	files := []domain.SeenFile{{Name: "Movie.mkv"}}
	plan := []mediafiles.PlanStep{{SourceFile: "Movie.mkv", Method: "refuse", Why: []string{"does not match the Title: Dune"}}}
	if got := im.nothingPlaced(domain.Downloader{}, files, plan); got != "no file in the download matched this movie; match the files by hand" {
		t.Fatalf("movie: %q", got)
	}
	if got := im.nothingMatched(domain.Downloader{}, files, false, plan); got != "no file in the download matched an episode; match the files by hand" {
		t.Fatalf("episode: %q", got)
	}
}
