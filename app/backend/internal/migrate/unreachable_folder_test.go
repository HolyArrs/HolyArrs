package migrate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func reloadedMeta(l *moveLab) {
	l.mv.Library.Metadata = &metadata.Static{Movies: []metadata.Movie{
		{Name: "The Matrix", Year: 1999, IDs: map[string]string{"tmdb": "603"}},
		{Name: "The Matrix Reloaded", Year: 2003, IDs: map[string]string{"tmdb": "604"}},
	}}
}

const reloaded = "title:movie:tmdb:604"

// A title under an old root folder this server cannot see comes over as
// wanted, into a Library folder here of the same kind, and its line says it
// needs attention. The dry run says exactly that first.
func TestMoveTitleUnderUnreachableRootComesOverWanted(t *testing.T) {
	l := newMoveLabWith(t, func(radarr, _, _ *Snapshot) {
		radarr.Folders = append(radarr.Folders, Folder{Path: "/movies-other"})
		radarr.Titles = append(radarr.Titles, Title{SourceID: 2, Kind: "movie", Name: "The Matrix Reloaded", Year: 2003,
			IDs: map[string]string{"tmdb": "604"}, Path: "/movies-other/The Matrix Reloaded (2003)", Monitored: true, ProfileID: 2})
	})
	reloadedMeta(l)
	rep := l.mv.Run(context.Background(), 0, l.plan(t), true, domain.Actor{Kind: "user", ID: 1}, nil)
	dry := lineOf(t, rep, reloaded)
	if dry.Result != ResCreate || dry.Attention != AttnUnreachable || dry.Folder != "/movies-other" || !strings.Contains(dry.Detail, "needs attention") || !strings.Contains(dry.Detail, filepath.ToSlash(l.movies)) {
		t.Fatalf("dry %+v", dry)
	}
	mv := l.move(t, Choices{})
	got := lineOf(t, mv.Report, reloaded)
	if got.Result != ResCreated || got.Attention != AttnUnreachable || !strings.Contains(got.Detail, "needs attention") || !strings.Contains(got.Detail, filepath.ToSlash(l.movies)) {
		t.Fatalf("move %+v", got)
	}
	if n := l.count(t, "SELECT count(*) FROM titles WHERE json_extract(source_ids, '$.tmdb') = '604' AND state = 'active'"); n != 1 {
		t.Fatalf("title not added: %d", n)
	}
}

// A folder that is on this server but left unchecked keeps its titles
// behind (option A): nothing is added elsewhere, so nothing is downloaded
// twice. The dry run says the same, and so does a wanted title with no
// folder of its own.
func TestMoveTitleUnderUncheckedFolderIsLeftOut(t *testing.T) {
	l := newMoveLabWith(t, func(radarr, _, _ *Snapshot) {
		radarr.Titles = append(radarr.Titles, Title{SourceID: 2, Kind: "movie", Name: "The Matrix Reloaded", Year: 2003,
			IDs: map[string]string{"tmdb": "604"}, Path: "/movies/The Matrix Reloaded (2003)", Monitored: true, ProfileID: 2})
	})
	reloadedMeta(l)
	ch := Choices{PathRules: l.rules, People: l.people, Skip: []string{GroupFolders}}
	rep := l.mv.Run(context.Background(), 0, l.planWith(t, ch), true, domain.Actor{Kind: "user", ID: 1}, nil)
	for _, key := range []string{"title:movie:tmdb:603", reloaded} {
		got := lineOf(t, rep, key)
		if got.Result != ResSkipped || got.Attention != AttnUnchecked || got.Folder != filepath.ToSlash(l.movies) || !strings.Contains(got.Detail, `set to "Don't bring them"`) {
			t.Fatalf("dry %s %+v", key, got)
		}
	}
	mv := l.move(t, ch)
	for _, key := range []string{"title:movie:tmdb:603", reloaded} {
		if got := lineOf(t, mv.Report, key); got.Result != ResSkipped || got.Attention != AttnUnchecked {
			t.Fatalf("move %s %+v", key, got)
		}
	}
	if n := l.count(t, "SELECT count(*) FROM titles WHERE state = 'active'"); n != 0 {
		t.Fatalf("%d titles added from an unchecked folder", n)
	}
}

// A root folder written with a trailing slash is the same folder.
func TestMoveRootFolderWithTrailingSlash(t *testing.T) {
	l := newMoveLabWith(t, func(radarr, _, _ *Snapshot) {
		radarr.Folders = []Folder{{Path: "/movies/"}}
	})
	rep := l.mv.Run(context.Background(), 0, l.plan(t), true, domain.Actor{Kind: "user", ID: 1}, nil)
	if got := lineOf(t, rep, "title:movie:tmdb:603"); got.Result != ResAdopt || got.Attention != "" {
		t.Fatalf("dry %+v", got)
	}
	mv := l.move(t, Choices{})
	if got := lineOf(t, mv.Report, "title:movie:tmdb:603"); got.Result != ResAdopted || got.Attention != "" {
		t.Fatalf("move %+v", got)
	}
}

// Two old root folders that map to one folder here make one Library
// folder, and the titles of both come over into it.
func TestMoveTwoRootsToOneFolder(t *testing.T) {
	l := newMoveLabWith(t, func(radarr, _, _ *Snapshot) {
		radarr.Folders = append(radarr.Folders, Folder{Path: "/films"})
		radarr.Titles = append(radarr.Titles, Title{SourceID: 2, Kind: "movie", Name: "The Matrix Reloaded", Year: 2003,
			IDs: map[string]string{"tmdb": "604"}, Path: "/films/The Matrix Reloaded (2003)", Monitored: true, ProfileID: 2})
	})
	reloadedMeta(l)
	l.rules = append(l.rules, PathRule{From: "/films", To: filepath.ToSlash(l.movies)})
	rep := l.mv.Run(context.Background(), 0, l.plan(t), true, domain.Actor{Kind: "user", ID: 1}, nil)
	if got := lineOf(t, rep, reloaded); got.Result != ResCreate || got.Attention != "" {
		t.Fatalf("dry %+v", got)
	}
	mv := l.move(t, Choices{})
	for key, want := range map[string]string{"title:movie:tmdb:603": ResAdopted, reloaded: ResCreated} {
		if got := lineOf(t, mv.Report, key); got.Result != want || got.Attention != "" {
			t.Fatalf("move %s %+v", key, got)
		}
	}
	if n := l.count(t, "SELECT count(*) FROM library_folders"); n != 1 {
		t.Fatalf("%d library folders", n)
	}
}

// A root folder left at "Don't bring them" that is on this server keeps
// its titles behind, wanted ones too, and the folder is not added; one
// this server cannot see sends its titles over as wanted only while it is
// not set to "Don't bring them".
func TestMoveLeaveFolderKeepsTitlesBehind(t *testing.T) {
	l := newMoveLabWith(t, func(radarr, _, _ *Snapshot) {
		radarr.Folders = append(radarr.Folders, Folder{Path: "/movies-gone"})
		radarr.Titles = append(radarr.Titles,
			Title{SourceID: 2, Kind: "movie", Name: "The Matrix Reloaded", Year: 2003, IDs: map[string]string{"tmdb": "604"},
				Path: "/movies/The Matrix Reloaded (2003)", Monitored: true, ProfileID: 2},
			Title{SourceID: 3, Kind: "movie", Name: "The Matrix Revolutions", Year: 2003, IDs: map[string]string{"tmdb": "605"},
				Path: "/movies-gone/The Matrix Revolutions (2003)", Monitored: true, ProfileID: 2})
	})
	l.mv.Library.Metadata = &metadata.Static{Movies: []metadata.Movie{
		{Name: "The Matrix", Year: 1999, IDs: map[string]string{"tmdb": "603"}},
		{Name: "The Matrix Reloaded", Year: 2003, IDs: map[string]string{"tmdb": "604"}},
		{Name: "The Matrix Revolutions", Year: 2003, IDs: map[string]string{"tmdb": "605"}},
	}}
	// /movies is on this server (through the rule) and left out; a second
	// movie folder here gives the unreachable one somewhere to go.
	other := filepath.Join(t.TempDir(), "other")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := library.AddFolder(context.Background(), l.st, library.FolderInput{Path: other, Kind: "movie", Label: "other"}, nil, domain.System); err != nil {
		t.Fatal(err)
	}
	ch := Choices{PathRules: l.rules, People: l.people, LeaveFolders: []string{"/movies/"}}
	check := func(stage string, rep *Report, dry bool) {
		t.Helper()
		for _, key := range []string{"title:movie:tmdb:603", reloaded} {
			if got := lineOf(t, rep, key); got.Result != ResSkipped || got.Attention != AttnUnchecked || !strings.Contains(got.Detail, `set to "Don't bring them"`) {
				t.Fatalf("%s %s %+v", stage, key, got)
			}
		}
		if got := lineOf(t, rep, "folder:"+filepath.ToSlash(l.movies)); got.Result != ResSkipped || got.Attention != AttnUnchecked {
			t.Fatalf("%s folder %+v", stage, got)
		}
		want := ResCreated
		if dry {
			want = ResCreate
		}
		if got := lineOf(t, rep, "title:movie:tmdb:605"); got.Result != want || got.Attention != AttnUnreachable || !strings.Contains(got.Detail, filepath.ToSlash(other)) {
			t.Fatalf("%s unreachable %+v", stage, got)
		}
	}
	check("dry", l.mv.Run(context.Background(), 0, l.planWith(t, ch), true, domain.Actor{Kind: "user", ID: 1}, nil), true)
	check("move", l.move(t, ch).Report, false)
	if n := l.count(t, "SELECT count(*) FROM titles WHERE state = 'active'"); n != 1 {
		t.Fatalf("%d titles, want only the unreachable one", n)
	}
	if n := l.count(t, "SELECT count(*) FROM library_folders WHERE path = ?", filepath.Clean(l.movies)); n != 0 {
		t.Fatal("the left-out folder was added")
	}
}
