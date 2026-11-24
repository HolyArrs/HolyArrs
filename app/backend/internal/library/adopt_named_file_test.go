package library

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// The file an old app recorded becomes the movie's File even when a bigger
// video sits beside it, and a rescan keeps it as the movie's copy. A name
// that is missing, an extra or outside the folder leaves the biggest video,
// as adopting picked before.
func TestAdoptKeepsTheNamedFile(t *testing.T) {
	const dir = "Heat (1995)"
	for _, c := range []struct{ name, want string }{
		{"Heat.1995.1080p.mkv", "Heat.1995.1080p.mkv"},
		{"", "Heat.1995.2160p.mkv"},
		{"Missing.1995.mkv", "Heat.1995.2160p.mkv"},
		{"Heat (1995)-trailer.mkv", "Heat.1995.2160p.mkv"},
		{"../Heat.1995.1080p.mkv", "Heat.1995.2160p.mkv"},
		{`..\Heat (1995)\Heat.1995.1080p.mkv`, "Heat.1995.2160p.mkv"},
		{"Extras/Heat.1995.1080p.mkv", "Heat.1995.2160p.mkv"},
	} {
		s := open(t)
		lf := addFolder(t, s)
		writeFile(t, filepath.Join(lf.Path, dir, "Heat.1995.1080p.mkv"), strings.Repeat("h", 100))
		writeFile(t, filepath.Join(lf.Path, dir, "Heat.1995.2160p.mkv"), strings.Repeat("u", 400))
		writeFile(t, filepath.Join(lf.Path, dir, "Heat (1995)-trailer.mkv"), strings.Repeat("t", 50))
		writeFile(t, filepath.Join(lf.Path, dir, "Extras", "Heat.1995.1080p.mkv"), strings.Repeat("e", 900))
		svc := &Service{Store: s, Metadata: adoptTMDB, Now: func() time.Time { return now }}
		ctx := context.Background()
		res, err := svc.Adopt(ctx, lf.ID, []AdoptChoice{{Folder: dir, TMDB: "949", File: c.name}}, domain.System)
		if err != nil || len(res) != 1 || res[0].TitleID == 0 || res[0].Error != "" {
			t.Fatalf("name %q: %+v %v", c.name, res, err)
		}
		mainFile := func() string {
			sl := slotsOf(t, s, res[0].TitleID)[0]
			if sl.FileID == nil {
				return ""
			}
			var f mediafiles.File
			if err := database.Get(ctx, s.Read(), "files", *sl.FileID, &f); err != nil {
				t.Fatal(err)
			}
			return f.RelativePath
		}
		if got := mainFile(); got != dir+"/"+c.want {
			t.Fatalf("name %q: the movie's file is %q, want %q", c.name, got, c.want)
		}
		if _, err := svc.Rescan(ctx, domain.System); err != nil {
			t.Fatal(err)
		}
		if got := mainFile(); got != dir+"/"+c.want {
			t.Fatalf("name %q: after a rescan the movie's file is %q, want %q", c.name, got, c.want)
		}
	}
}

// Where links are followed, the named file may be a link to the movie, as
// listing the folder counts it; where they are not, the link is passed
// over and the biggest plain video is the File, as before.
func TestAdoptKeepsANamedLinkedFile(t *testing.T) {
	for _, follow := range []bool{true, false} {
		s := open(t)
		lf := addFolder(t, s)
		target := filepath.Join(t.TempDir(), "Heat.1995.1080p.mkv")
		writeFile(t, target, strings.Repeat("h", 100))
		writeFile(t, filepath.Join(lf.Path, "Heat (1995)", "Heat.1995.2160p.mkv"), strings.Repeat("u", 400))
		if err := os.Symlink(target, filepath.Join(lf.Path, "Heat (1995)", "Heat.1995.1080p.mkv")); err != nil {
			t.Skip("symlinks not available:", err)
		}
		old := importer.FollowLinks
		importer.FollowLinks = follow
		svc := &Service{Store: s, Metadata: adoptTMDB, Now: func() time.Time { return now }}
		res, err := svc.Adopt(context.Background(), lf.ID, []AdoptChoice{{Folder: "Heat (1995)", TMDB: "949", File: "Heat.1995.1080p.mkv"}}, domain.System)
		importer.FollowLinks = old
		if err != nil || len(res) != 1 || res[0].TitleID == 0 || res[0].Error != "" {
			t.Fatalf("follow %v: %+v %v", follow, res, err)
		}
		want := "Heat (1995)/Heat.1995.1080p.mkv"
		if !follow {
			want = "Heat (1995)/Heat.1995.2160p.mkv"
		}
		sl := slotsOf(t, s, res[0].TitleID)[0]
		var f mediafiles.File
		if sl.FileID == nil || database.Get(context.Background(), s.Read(), "files", *sl.FileID, &f) != nil || f.RelativePath != want {
			t.Fatalf("follow %v: the movie's file is %+v, want %s", follow, f.RelativePath, want)
		}
	}
}

// A path an old app recorded in a subfolder of a movie folder names that
// file only where listing the folder reaches it, never an extra, a sample
// or a path that leaves the folder; MovieFeatures lists only the cuts
// directly in the folder, however many extras sit in subfolders.
func TestRecordedMovieVideoInASubfolder(t *testing.T) {
	root := t.TempDir()
	const dir = "Heat (1995)"
	for _, p := range []string{"Disc1/Heat.mkv", "Heat.mkv", "Featurettes/Heat.mkv", "Sample/Heat.mkv", ".hidden/Heat.mkv",
		"a/b/c/d/Heat.mkv", "a/b/c/d/e/Heat.mkv", "Disc1/Heat.1995.trailer.mkv"} {
		writeFile(t, filepath.Join(root, dir, filepath.FromSlash(p)), "x")
	}
	for rel, want := range map[string]string{
		"Disc1/Heat.mkv":              dir + "/Disc1/Heat.mkv",
		"Heat.mkv":                    dir + "/Heat.mkv",
		"a/b/c/d/Heat.mkv":            dir + "/a/b/c/d/Heat.mkv",
		"a/b/c/d/e/Heat.mkv":          "",
		"Featurettes/Heat.mkv":        "",
		"Sample/Heat.mkv":             "",
		".hidden/Heat.mkv":            "",
		"Disc1/Heat.1995.trailer.mkv": "",
		"Disc1/../Heat.mkv":           "",
		"../Heat (1995)/Heat.mkv":     "",
		`Disc1\Heat.mkv`:              "",
		"/Disc1/Heat.mkv":             "",
	} {
		v, ok := RecordedMovieVideo(root, dir, rel)
		if ok != (want != "") || v.Path != want {
			t.Errorf("%q: %q %v, want %q", rel, v.Path, ok, want)
		}
	}
	writeFile(t, filepath.Join(root, dir, "Heat (1995)-trailer.mkv"), "x")
	writeFile(t, filepath.Join(root, dir, "Heat.1995.trailer.mkv"), "x")
	if got := MovieFeatures(root, dir, 5); len(got) != 1 || got[0].Path != dir+"/Heat.mkv" {
		t.Fatalf("features %+v", got)
	}
}

// A recorded path through a linked folder is refused: listing the folder
// never follows one, so an extra reached that way is never the movie.
func TestRecordedMovieVideoNeverFollowsALinkedFolder(t *testing.T) {
	root := t.TempDir()
	const dir = "Heat (1995)"
	writeFile(t, filepath.Join(root, dir, "Extras", "Feature.mkv"), "x")
	if err := os.Symlink("Extras", filepath.Join(root, dir, "Disc1")); err != nil {
		t.Skipf("no links here: %v", err)
	}
	if v, ok := RecordedMovieVideo(root, dir, "Disc1/Feature.mkv"); ok {
		t.Fatalf("took %q through a linked folder", v.Path)
	}
}
