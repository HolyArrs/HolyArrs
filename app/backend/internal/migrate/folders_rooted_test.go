package migrate

import (
	"os"
	"path/filepath"
	"testing"
)

// The folder browser and search stay inside the roots: a link out of a
// root, or a path outside every root, is neither listed nor looked up.
func TestRootedStaysInside(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	for _, d := range []string{filepath.Join(root, "Movies", "Dune (2021)"), filepath.Join(outside, "secret")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	linked := os.Symlink(outside, filepath.Join(root, "escape")) == nil
	r := newRooted([]string{filepath.ToSlash(root)})
	defer r.close()
	slash := func(parts ...string) string { return filepath.ToSlash(filepath.Join(parts...)) }
	for _, tc := range []struct {
		name string
		path string
		dir  bool
		kids int
		skip bool
	}{
		{"inside", slash(root, "Movies"), true, 1, false},
		{"root itself", slash(root), true, 1, false},
		{"outside every root", slash(outside), false, 0, false},
		{"dot-dot out", slash(root) + "/../" + filepath.Base(outside), false, 0, false},
		{"link out of the root", slash(root, "escape"), false, 0, !linked},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.skip {
				t.Skip("this system cannot make links")
			}
			if got := r.dir(tc.path); got != tc.dir {
				t.Errorf("dir = %v, want %v", got, tc.dir)
			}
			if got := len(r.sub(tc.path, 10)); got != tc.kids {
				t.Errorf("%d folders listed, want %d", got, tc.kids)
			}
		})
	}
}

// MarkFound only looks inside the roots and only probes writes there: a
// plan folder outside them is marked missing and nothing is written to it.
func TestMarkFoundStaysInRoots(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	for _, d := range []string{filepath.Join(root, "Dune (2021)"), filepath.Join(outside, "Dune (2021)")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name    string
		path    string
		missing bool
		found   int
	}{
		{"inside a root", filepath.ToSlash(root), false, 1},
		{"outside the roots", filepath.ToSlash(outside), true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &FolderPlan{OldPath: "/movies", Path: tc.path, Kind: "movie"}
			p := &Plan{Groups: []Group{{Name: "folders", Items: []Item{{Folder: f}}}, {Name: "titles", Items: []Item{{Title: &TitlePlan{Library: tc.path, Folder: "Dune (2021)", hasFiles: true}}}}}}
			r := newRooted([]string{filepath.ToSlash(root)})
			MarkFound(p, r)
			r.close()
			if f.Missing != tc.missing || f.Found != tc.found {
				t.Errorf("missing=%v found=%d, want %v %d", f.Missing, f.Found, tc.missing, tc.found)
			}
			es, _ := os.ReadDir(outside)
			if len(es) != 1 {
				t.Errorf("something was written outside the roots: %d entries", len(es))
			}
		})
	}
}
