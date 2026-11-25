package importer

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A path pair with "Place a link" on puts a symlink in the Library that
// points at the download, instead of a hardlink, move or copy. Renaming or
// deleting the Library entry never touches the file it points at.
func TestPlaceLinkFromPathPair(t *testing.T) {
	cases := []struct {
		name     string
		link     bool // the pair's switch
		inside   bool // the download sits under the pair's folder
		usenet   bool // a Usenet download would otherwise move
		wantLink bool
	}{
		{name: "switch on, inside the pair", link: true, inside: true, wantLink: true},
		{name: "switch on, Usenet does not move", link: true, inside: true, usenet: true, wantLink: true},
		{name: "switch off keeps hardlink or copy", link: false, inside: true},
		{name: "switch on, file outside the pair", link: true, inside: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mount, other, lib := t.TempDir(), t.TempDir(), t.TempDir()
			dir := other
			if tc.inside {
				dir = mount
			}
			src := filepath.Join(dir, "job", "Movie.2020.mkv")
			if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(src, []byte("video bytes"), 0o644); err != nil {
				t.Fatal(err)
			}
			dr := domain.Downloader{PathMap: []domain.PathMap{{Theirs: "/mnt/remote", Ours: mount, Link: tc.link}}}
			if tc.usenet {
				dr.Protocol = "usenet"
			}
			plan := []mediafiles.PlanStep{{SourceFile: src, TargetPath: filepath.Join(lib, "Movie (2020)", "Movie (2020).mkv"), Method: "hardlink", Move: tc.usenet}}
			markLinks(plan, dr)
			if plan[0].Symlink != tc.wantLink {
				t.Fatalf("Symlink = %v, want %v", plan[0].Symlink, tc.wantLink)
			}
			if tc.wantLink && plan[0].Move {
				t.Fatal("a linked file must never be moved out of the download")
			}
			if !tc.wantLink {
				return
			}
			called := false
			ops := Ops{
				Link: func(string, string) error { called = true; return errors.New("no") },
				Copy: func(string, string) error { called = true; return errors.New("no") },
				Move: func(string, string) error { called = true; return errors.New("no") },
			}
			r := place(plan[0], ops, time.Time{})
			if err := os.Symlink(src, filepath.Join(t.TempDir(), "probe")); err != nil {
				if r.Outcome != "failed" {
					t.Fatalf("no symlink support: want failed, got %+v", r)
				}
				t.Skip("symlinks not available:", err)
			}
			if r.Outcome != "placed" || !r.Linked || called {
				t.Fatalf("want placed as a link without copy/hardlink/move, got %+v (ops called %v)", r, called)
			}
			fi, err := os.Lstat(plan[0].TargetPath)
			if err != nil || fi.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("want a symlink at the target, got %v %v", fi, err)
			}
			if to, _ := os.Readlink(plan[0].TargetPath); !filepath.IsAbs(to) {
				t.Fatalf("link must hold an absolute path, got %q", to)
			}
			if got := mustReal(t, plan[0].TargetPath); got != mustReal(t, src) {
				t.Fatalf("link points to %q, want %q", got, src)
			}
			// Placing again finds its own link and leaves it.
			if again := place(plan[0], ops, time.Time{}); again.Outcome != "placed" || !again.Linked {
				t.Fatalf("second place: %+v", again)
			}

			// Rename keeps the link and its target.
			renamed := filepath.Join(lib, "Movie (2020)", "Movie (2020) Bluray-1080p.mkv")
			if err := renameNoReplace(plan[0].TargetPath, renamed); err != nil {
				t.Fatal(err)
			}
			if fi, err := os.Lstat(renamed); err != nil || fi.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("renamed entry is no longer a link: %v %v", fi, err)
			}
			if b, err := os.ReadFile(renamed); err != nil || string(b) != "video bytes" {
				t.Fatalf("renamed link does not read the download: %q %v", b, err)
			}

			// Deleting the entry, or its whole folder, never removes the target.
			if err := os.Remove(renamed); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(src); err != nil {
				t.Fatalf("delete removed the link target: %v", err)
			}
			if r := place(plan[0], ops, time.Time{}); r.Outcome != "placed" {
				t.Fatal(r)
			}
			if err := os.RemoveAll(filepath.Join(lib, "Movie (2020)")); err != nil {
				t.Fatal(err)
			}
			if b, err := os.ReadFile(src); err != nil || string(b) != "video bytes" {
				t.Fatalf("folder delete touched the link target: %q %v", b, err)
			}
		})
	}
}

// A link whose target is gone is refused at place time with a clear reason,
// and a different file already at the target is never replaced.
func TestPlaceLinkRefusals(t *testing.T) {
	mount, lib := t.TempDir(), t.TempDir()
	src := filepath.Join(mount, "a.mkv")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	taken := filepath.Join(lib, "a.mkv")
	if err := os.WriteFile(taken, []byte("other"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := place(mediafiles.PlanStep{SourceFile: src, TargetPath: taken, Method: "hardlink", Symlink: true}, Ops{}, time.Time{})
	if r.Outcome != "refused" {
		t.Fatalf("want refused when another file sits at the target, got %+v", r)
	}
	if b, _ := os.ReadFile(taken); string(b) != "other" {
		t.Fatal("the existing file was changed")
	}
	r = place(mediafiles.PlanStep{SourceFile: filepath.Join(mount, "gone.mkv"), TargetPath: filepath.Join(lib, "b.mkv"), Method: "hardlink", Symlink: true}, Ops{}, time.Time{})
	if r.Outcome != "failed" {
		t.Fatalf("want failed for a missing source, got %+v", r)
	}
}

// Once a pair places links, the Library scan must count those links as files
// even without HOLYARRS_FOLLOW_LINKS.
func TestLinkPairsFollowLinks(t *testing.T) {
	mount, lib := t.TempDir(), t.TempDir()
	src := filepath.Join(mount, "a.mkv")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	ln := filepath.Join(lib, "a.mkv")
	if err := os.Symlink(src, ln); err != nil {
		t.Skip("symlinks not available:", err)
	}
	old := FollowLinks
	defer func() { FollowLinks = old; linkPairsOn.Store(false) }()
	FollowLinks = false
	linkPairsOn.Store(false)
	if _, ok := LinkedFile(ln); ok {
		t.Fatal("links must not be followed by default")
	}
	NoteLinkPairs([]domain.Downloader{{PathMap: []domain.PathMap{{Theirs: "/x", Ours: mount}}}})
	if _, ok := LinkedFile(ln); ok {
		t.Fatal("a pair without the switch must not turn links on")
	}
	NoteLinkPairs([]domain.Downloader{{PathMap: []domain.PathMap{{Theirs: "/x", Ours: mount, Link: true}}}})
	if _, ok := LinkedFile(ln); !ok {
		t.Fatal("a pair with the switch must make links count as files")
	}
}
