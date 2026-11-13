package migrate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

const (
	severance = "title:show:tvdb:371980"
	expanse   = "title:show:tvdb:280619"
)

// mediaLab is a release tester's shape: Library folders here at
// .../data/media/tv and .../data/media/movies, and the old Sonarr and Radarr
// keeping their titles under /old/tv and /old/movies, which this server
// cannot see.
func mediaLab(t *testing.T) (l *moveLab, tv, movies string) {
	t.Helper()
	l = newMoveLabWith(t, func(radarr, _, _ *Snapshot) {
		radarr.Folders = []Folder{{Path: "/old/movies"}}
		radarr.Titles[0].Path = "/old/movies/The Matrix (1999)"
		radarr.Titles = append(radarr.Titles, Title{SourceID: 2, Kind: "movie", Name: "The Matrix Reloaded", Year: 2003,
			IDs: map[string]string{"tmdb": "604"}, Path: "/old/movies/The Matrix Reloaded (2003)", Monitored: true, ProfileID: 2})
	})
	reloadedMeta(l)
	ctx := context.Background()
	sonarr := &Snapshot{Kind: Sonarr, Version: "4.0.15", Via: ViaAPI, Folders: []Folder{{Path: "/old/tv"}}, Titles: []Title{
		{SourceID: 1, Kind: "show", Name: "Severance", Year: 2022, IDs: map[string]string{"tvdb": "371980"}, Path: "/old/tv/Severance", Monitored: true},
		{SourceID: 2, Kind: "show", Name: "The Expanse", Year: 2015, IDs: map[string]string{"tvdb": "280619"}, Path: "/old/tv/The Expanse", Monitored: true},
	}}
	sonarr.finish()
	saved, err := Save(ctx, l.st, 1, sonarr)
	if err != nil {
		t.Fatal(err)
	}
	l.snaps = append(l.snaps, saved.ID)
	media := filepath.Join(t.TempDir(), "data", "media")
	tv, movies = filepath.Join(media, "tv"), filepath.Join(media, "movies")
	for _, f := range []library.FolderInput{{Path: tv, Kind: "show", Label: "tv"}, {Path: movies, Kind: "movie", Label: "movies"}} {
		if err := os.MkdirAll(f.Path, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := library.AddFolder(ctx, l.st, f, nil, domain.System); err != nil {
			t.Fatal(err)
		}
	}
	return l, tv, movies
}

func previewDetail(p *Plan, key string) string {
	for _, g := range p.Groups {
		for _, it := range g.Items {
			if it.Key == key {
				return it.Detail
			}
		}
	}
	return ""
}

// A folder this server cannot see, set to "Don't bring them", keeps its
// titles behind: the preview, the dry run and the import all say so, and
// the folder itself is left out rather than "not found".
func TestCannotSeeFolderSetToDontBringStaysBehind(t *testing.T) {
	l, _, _ := mediaLab(t)
	ctx := context.Background()
	ch := Choices{People: l.people, LeaveFolders: []string{"/old/tv", "/old/movies"}}
	p := l.planWith(t, ch)
	l.mv.PreviewTitles(ctx, p)
	dry := l.mv.Run(ctx, 0, l.planWith(t, ch), true, domain.Actor{Kind: "user", ID: 1}, nil)
	for key, old := range map[string]string{severance: "/old/tv", expanse: "/old/tv", "title:movie:tmdb:603": "/old/movies", reloaded: "/old/movies"} {
		got := lineOf(t, dry, key)
		if got.Result != ResSkipped || got.Attention != AttnUnchecked || got.Folder != old || !strings.Contains(got.Detail, `set to "Don't bring them"`) {
			t.Fatalf("dry %s %+v", key, got)
		}
		if d := previewDetail(p, key); d != got.Detail {
			t.Fatalf("%s: preview says %q, the dry run %q", key, d, got.Detail)
		}
	}
	if got := lineOf(t, dry, "folder:/old/tv"); got.Result != ResSkipped || got.Attention != AttnUnchecked {
		t.Fatalf("folder %+v", got)
	}
	mv := l.move(t, Choices{PathRules: []PathRule{}, People: l.people, LeaveFolders: []string{"/old/movies"}})
	for _, key := range []string{"title:movie:tmdb:603", reloaded} {
		if got := lineOf(t, mv.Report, key); got.Result != ResSkipped || got.Attention != AttnUnchecked {
			t.Fatalf("move %s %+v", key, got)
		}
	}
	if n := l.count(t, "SELECT count(*) FROM titles WHERE state = 'active'"); n != 0 {
		t.Fatalf("%d titles came over from a folder set to \"Don't bring them\"", n)
	}
}

// Left at its default, a folder this server cannot see still sends its
// titles over as wanted, needing attention.
func TestCannotSeeFolderByDefaultComesOverNeedingAttention(t *testing.T) {
	l, tv, _ := mediaLab(t)
	ctx := context.Background()
	dry := l.mv.Run(ctx, 0, l.planWith(t, Choices{People: l.people}), true, domain.Actor{Kind: "user", ID: 1}, nil)
	for _, key := range []string{severance, expanse} {
		if got := lineOf(t, dry, key); got.Result != ResCreate || got.Attention != AttnUnreachable || got.Folder != "/old/tv" || !strings.Contains(got.Detail, filepath.ToSlash(tv)) {
			t.Fatalf("dry %s %+v", key, got)
		}
	}
}

// The plan marks each old root folder this server cannot see, so the
// folder step can word its choices for what they do.
func TestPlanMarksFoldersServerCannotSee(t *testing.T) {
	l, _, _ := mediaLab(t)
	p := l.planWith(t, Choices{People: l.people})
	MarkUnseen(p)
	n := 0
	for _, it := range p.Groups[groupIndex(GroupFolders)].Items {
		if it.Folder != nil && strings.HasPrefix(it.Folder.OldPath, "/old/") {
			n++
			if !it.Folder.Unseen {
				t.Fatalf("%s not marked", it.Folder.OldPath)
			}
		}
	}
	if n != 2 {
		t.Fatalf("%d old folders", n)
	}
}
