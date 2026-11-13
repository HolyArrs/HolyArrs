package migrate

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
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
		if got.Result != ResSkipped || got.Attention != AttnUnchecked || got.Folder != filepath.ToSlash(l.movies) || !strings.Contains(got.Detail, "left unchecked") {
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
