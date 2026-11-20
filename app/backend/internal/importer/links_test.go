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

// A file inside a link folder is placed as a link to it and never copied;
// the same file outside one is placed as before.
func TestPlaceFromLinkPath(t *testing.T) {
	mount, other, lib := t.TempDir(), t.TempDir(), t.TempDir()
	inMount := filepath.Join(mount, "Movie.2020", "Movie.2020.mkv")
	if err := os.MkdirAll(filepath.Dir(inMount), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inMount, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(other, "Movie.2020.mkv")
	if err := os.WriteFile(outside, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldPaths, oldFollow := LinkPaths, FollowLinks
	defer func() { LinkPaths, FollowLinks = oldPaths, oldFollow }()
	LinkPaths, FollowLinks = []string{mount}, true

	target := filepath.Join(lib, "Movie (2020)", "Movie (2020).mkv")
	r := place(mediafiles.PlanStep{SourceFile: inMount, TargetPath: target, Method: "hardlink", Move: true}, Ops{}, time.Time{})
	if r.Outcome != "placed" {
		t.Fatalf("want placed, got %+v", r)
	}
	fi, err := os.Lstat(target)
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("want a symlink at the target, got %v %v", fi, err)
	}
	if got, _ := os.Readlink(target); got != inMount {
		t.Fatalf("link points to %q, want %q", got, inMount)
	}
	if _, err := os.Stat(inMount); err != nil {
		t.Fatalf("the download must stay: %v", err)
	}

	// A second placement at the same target is refused, not overwritten.
	if r := place(mediafiles.PlanStep{SourceFile: inMount, TargetPath: target, Method: "hardlink"}, Ops{}, time.Time{}); r.Outcome == "failed" {
		t.Fatalf("want refused or placed again, got %+v", r)
	}

	// Outside a link folder nothing changes: a plain hardlink.
	t2 := filepath.Join(lib, "Other (2020)", "Other (2020).mkv")
	r = place(mediafiles.PlanStep{SourceFile: outside, TargetPath: t2, Method: "hardlink"}, Ops{Link: os.Link, Copy: copyFile}, time.Time{})
	if r.Outcome != "placed" {
		t.Fatalf("want placed, got %+v", r)
	}
	if fi, err := os.Lstat(t2); err != nil || fi.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("a file outside the link folder must not become a symlink: %v %v", fi, err)
	}
}

func TestInLinkPath(t *testing.T) {
	old := LinkPaths
	defer func() { LinkPaths = old }()
	LinkPaths = parseLinkPaths(" /mnt/a/ , relative/ignored,/mnt/b")
	if len(LinkPaths) != 2 {
		t.Fatalf("want two absolute folders, got %v", LinkPaths)
	}
	for p, want := range map[string]bool{
		"/mnt/a/x/y.mkv": true, "/mnt/b/y.mkv": true,
		"/mnt/a": false, "/mnt/ab/y.mkv": false, "/mnt/c/y.mkv": false, "/mnt/a/../c/y.mkv": false,
	} {
		if got := InLinkPath(p); got != want {
			t.Errorf("InLinkPath(%q) = %v, want %v", p, got, want)
		}
	}
	LinkPaths = nil
	if InLinkPath("/mnt/a/x.mkv") {
		t.Error("no folders set: nothing is in a link folder")
	}
}
