package importer

import (
	"os"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"path/filepath"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A plan of refused names is a refusal, never files another app took.
func TestSourcesGoneIgnoresRefusedNames(t *testing.T) {
	root := t.TempDir()
	_, bad := candidates(root, []domain.SeenFile{{Name: filepath.Join(t.TempDir(), "Example.mp4"), Size: 1}})
	if len(bad) != 1 {
		t.Fatalf("leading-slash name not refused: %+v", bad)
	}
	if sourcesGone(root, bad, false) {
		t.Fatal("refused names read as removed by another app")
	}
}

// A Usenet job of archives only, taken by another app, is gone even though
// its folder stays; one archive still there is not. A torrent is never gone.
func TestSourcesGoneArchivesOnly(t *testing.T) {
	root := t.TempDir()
	plan := []mediafiles.PlanStep{
		{SourceFile: filepath.Join(root, "Show", "a.rar"), Method: "unpack"},
		{SourceFile: filepath.Join(root, "Show", "a.r00"), Method: "unpack"},
		{SourceFile: filepath.Join(root, "Show", "a.nfo"), Method: "skip"},
	}
	if sourcesGone(root, plan, false) {
		t.Fatal("a torrent with its files missing read as taken by another app")
	}
	if !sourcesGone(root, plan, true) {
		t.Fatal("archives-only job with every archive gone not counted as gone")
	}
	if err := os.MkdirAll(filepath.Join(root, "Show"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Show", "a.rar"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if sourcesGone(root, plan, true) {
		t.Fatal("an archive still there read as gone")
	}
	// A plan with a video judges by the video alone: a missing side file never counts.
	media := append(plan[:1:1], mediafiles.PlanStep{SourceFile: filepath.Join(root, "Show", "a.mkv"), Method: "move"})
	if !sourcesGone(root, media[1:], true) {
		t.Fatal("missing video not gone")
	}
}
