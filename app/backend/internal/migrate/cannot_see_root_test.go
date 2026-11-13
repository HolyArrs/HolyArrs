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

const oldMovies = "/data/media/mediamanager/movies"

// cannotSeeLab is the shape a release tester met: the old Radarr keeps its
// movies under a root this server cannot see, and the only movies Library
// folder here (also named "movies") holds none of them.
func cannotSeeLab(t *testing.T) (*moveLab, string) {
	t.Helper()
	l := newMoveLabWith(t, func(radarr, _, _ *Snapshot) {
		radarr.Folders = []Folder{{Path: oldMovies}}
		radarr.Titles[0].Path = oldMovies + "/The Matrix (1999)"
		radarr.Titles = append(radarr.Titles, Title{SourceID: 2, Kind: "movie", Name: "The Matrix Reloaded", Year: 2003,
			IDs: map[string]string{"tmdb": "604"}, Path: oldMovies + "/The Matrix Reloaded (2003)", Monitored: true, ProfileID: 2})
	})
	reloadedMeta(l)
	here := filepath.Join(t.TempDir(), "movies")
	if err := os.MkdirAll(here, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := library.AddFolder(context.Background(), l.st, library.FolderInput{Path: here, Kind: "movie", Label: "movies"}, nil, domain.System); err != nil {
		t.Fatal(err)
	}
	return l, here
}

// The page fills each suggested folder in; a folder the titles are not in
// is not suggested, so a folder this server cannot see, left at its
// default, sends them over as wanted needing attention instead of "added
// with no files".
func TestRootServerCannotSeeComesOverNeedingAttention(t *testing.T) {
	l, _ := cannotSeeLab(t)
	ctx := context.Background()
	sug := SuggestRules(ctx, l.st, l.planWith(t, Choices{People: l.people}))
	if len(sug) != 0 {
		t.Fatalf("suggested %v for a folder that holds none of the titles", sug)
	}
	ch := Choices{PathRules: sug, People: l.people}
	check := func(stage string, rep *Report, want string) {
		t.Helper()
		for _, key := range []string{"title:movie:tmdb:603", reloaded} {
			got := lineOf(t, rep, key)
			if got.Result != want || got.Attention != AttnUnreachable || got.Folder != oldMovies || !strings.Contains(got.Detail, "needs attention") {
				t.Fatalf("%s %s %+v", stage, key, got)
			}
		}
	}
	check("dry", l.mv.Run(ctx, 0, l.planWith(t, ch), true, domain.Actor{Kind: "user", ID: 1}, nil), ResCreate)
	check("move", l.move(t, ch).Report, ResCreated)
}

// A Library folder here that holds the titles' folders is still suggested
// for an old root of another name (the same folder, mounted elsewhere).
func TestSuggestRulesKeepsAFolderHoldingTheTitles(t *testing.T) {
	l, here := cannotSeeLab(t)
	if err := os.MkdirAll(filepath.Join(here, "The Matrix (1999)"), 0o755); err != nil {
		t.Fatal(err)
	}
	sug := SuggestRules(context.Background(), l.st, l.planWith(t, Choices{People: l.people}))
	if len(sug) != 1 || sug[0].From != oldMovies || sug[0].To != filepath.ToSlash(here) {
		t.Fatalf("suggested %v", sug)
	}
}

// The plan's titles list says what the import then does: a title under a
// folder left at "Don't bring them" that is on this server is left out, one
// under a folder this server cannot see, left at its default, needs
// attention.
func TestPreviewTitlesMatchesTheImport(t *testing.T) {
	ctx := context.Background()
	// /movies is on this server (through the rule); the second old root is not.
	l2 := newMoveLabWith(t, func(radarr, _, _ *Snapshot) {
		radarr.Folders = append(radarr.Folders, Folder{Path: oldMovies})
		radarr.Titles = append(radarr.Titles, Title{SourceID: 2, Kind: "movie", Name: "The Matrix Reloaded", Year: 2003,
			IDs: map[string]string{"tmdb": "604"}, Path: oldMovies + "/The Matrix Reloaded (2003)", Monitored: true, ProfileID: 2})
	})
	reloadedMeta(l2)
	other := filepath.Join(t.TempDir(), "other")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := library.AddFolder(ctx, l2.st, library.FolderInput{Path: other, Kind: "movie", Label: "other"}, nil, domain.System); err != nil {
		t.Fatal(err)
	}
	ch := Choices{PathRules: l2.rules, People: l2.people, LeaveFolders: []string{"/movies"}}
	p := l2.planWith(t, ch)
	l2.mv.PreviewTitles(ctx, p)
	rep := l2.mv.Run(ctx, 0, l2.planWith(t, ch), true, domain.Actor{Kind: "user", ID: 1}, nil)
	for key, want := range map[string]string{"title:movie:tmdb:603": `set to "Don't bring them"`, reloaded: "needs attention"} {
		var detail string
		for _, g := range p.Groups {
			for _, it := range g.Items {
				if it.Key == key {
					detail = it.Detail
				}
			}
		}
		if !strings.Contains(detail, want) || detail != lineOf(t, rep, key).Detail {
			t.Fatalf("%s: plan says %q, the dry run %q", key, detail, lineOf(t, rep, key).Detail)
		}
	}
	// A title that uses its folder where it is keeps the plan's own words.
	p = l2.planWith(t, Choices{PathRules: l2.rules, People: l2.people})
	l2.mv.PreviewTitles(ctx, p)
	for _, it := range p.Groups[groupIndex(GroupTitles)].Items {
		if it.Key == "title:movie:tmdb:603" && it.Detail != "uses the folder "+filepath.ToSlash(filepath.Join(l2.movies, "The Matrix (1999)"))+" where it is; Keep up to date off" {
			t.Fatalf("adopted title's plan line changed: %q", it.Detail)
		}
	}
}

func groupIndex(name string) int {
	for i, g := range GroupOrder {
		if g == name {
			return i
		}
	}
	return -1
}
