package importer

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A downloaded file that is a link is placed as a link when links are
// followed, and refused otherwise.
func TestPlaceFollowedLink(t *testing.T) {
	mount, dl, lib := t.TempDir(), t.TempDir(), t.TempDir()
	target := filepath.Join(mount, "real.mkv")
	if err := os.WriteFile(target, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dl, "Movie.2020.mkv")
	if err := os.Symlink(target, src); err != nil {
		t.Skip("symlinks not available:", err)
	}
	old := FollowLinks
	defer func() { FollowLinks = old }()
	step := mediafiles.PlanStep{SourceFile: src, TargetPath: filepath.Join(lib, "Movie (2020)", "Movie (2020).mkv"), Method: "hardlink"}

	FollowLinks = false
	if r := place(step, Ops{}, time.Time{}); r.Outcome != "refused" {
		t.Fatalf("strict default: want refused, got %+v", r)
	}
	FollowLinks = true
	if cands, bad := candidates(dl, []domain.SeenFile{{Name: "Movie.2020.mkv", Size: 5}}); len(cands) != 1 || len(bad) != 0 {
		t.Fatalf("want the link accepted as a download file, got %v %v", cands, bad)
	}
	r := place(step, Ops{}, time.Time{})
	if r.Outcome != "placed" {
		t.Fatalf("want placed, got %+v", r)
	}
	fi, err := os.Lstat(step.TargetPath)
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("want a link at the target, got %v %v", fi, err)
	}
	if got, _ := filepath.EvalSymlinks(step.TargetPath); got != mustReal(t, target) {
		t.Fatalf("link points to %q", got)
	}
}

func mustReal(t *testing.T, p string) string {
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
