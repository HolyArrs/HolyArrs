package importer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A missing side file next to a refused video is not files another app took,
// and the reason names the refused file.
func TestMixedNamesRefusalNotGone(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "Show"), 0o755); err != nil {
		t.Fatal(err)
	}
	cands, bad := candidates(root, []domain.SeenFile{{Name: filepath.Join(t.TempDir(), "x.S01E04.mkv"), Size: 1}, {Name: "x.S01E04.nfo", Size: 1}})
	plan := append([]mediafiles.PlanStep{}, bad...)
	for _, c := range cands {
		plan = append(plan, mediafiles.PlanStep{SourceFile: c.Path, Method: "skip"})
	}
	if len(bad) != 1 || len(plan) != 2 {
		t.Fatalf("plan %+v", plan)
	}
	if sourcesGone(root, plan, false) {
		t.Fatal("a missing side file read as removed by another app")
	}
	im := &Importer{SavePath: "/data"}
	for _, why := range []string{im.nothingPlaced(domain.Downloader{}, []domain.SeenFile{{Name: "a"}}, plan), im.nothingMatched(domain.Downloader{}, []domain.SeenFile{{Name: "a"}}, false, plan)} {
		if !strings.Contains(why, "x.S01E04.mkv") || !strings.Contains(why, "outside the download folder") {
			t.Fatalf("reason %q does not name the refused file", why)
		}
	}
}
