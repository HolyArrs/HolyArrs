package migrate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Brought into a Library folder here that holds none of its titles'
// folders, a folder this server cannot see sends them over as wanted and
// needing attention, not as plainly "added with no files yet".
func TestCannotSeeFolderMappedToEmptyLibraryFolderNeedsAttention(t *testing.T) {
	l, _, movies := mediaLab(t)
	ctx := context.Background()
	ch := Choices{PathRules: []PathRule{{From: "/old/movies", To: filepath.ToSlash(movies)}}, People: l.people}
	check := func(stage string, rep *Report, want string) {
		t.Helper()
		for _, key := range []string{"title:movie:tmdb:603", reloaded} {
			got := lineOf(t, rep, key)
			if got.Result != want || got.Attention != AttnUnreachable || got.Folder != "/old/movies" || !strings.Contains(got.Detail, "needs attention") || !strings.Contains(got.Detail, filepath.ToSlash(movies)) {
				t.Fatalf("%s %s %+v", stage, key, got)
			}
		}
	}
	p := l.planWith(t, ch)
	l.mv.PreviewTitles(ctx, p)
	check("dry", l.mv.Run(ctx, 0, l.planWith(t, ch), true, domain.Actor{Kind: "user", ID: 1}, nil), ResCreate)
	if !strings.Contains(previewDetail(p, reloaded), "needs attention") {
		t.Fatalf("preview %q", previewDetail(p, reloaded))
	}
	check("move", l.move(t, ch).Report, ResCreated)
	if n := l.count(t, "SELECT count(*) FROM titles WHERE state = 'active'"); n != 2 {
		t.Fatalf("%d titles, want both as wanted", n)
	}
}

// A title whose folder is there under the chosen Library folder uses it as
// before, and a wanted title next to it stays plainly "no files yet".
func TestCannotSeeFolderMappedToTheRealFolderUsesIt(t *testing.T) {
	l, _, movies := mediaLab(t)
	if err := os.MkdirAll(filepath.Join(movies, "The Matrix (1999)"), 0o755); err != nil {
		t.Fatal(err)
	}
	ch := Choices{PathRules: []PathRule{{From: "/old/movies", To: filepath.ToSlash(movies)}}, People: l.people}
	dry := l.mv.Run(context.Background(), 0, l.planWith(t, ch), true, domain.Actor{Kind: "user", ID: 1}, nil)
	if got := lineOf(t, dry, "title:movie:tmdb:603"); got.Result != ResAdopt || got.Attention != "" {
		t.Fatalf("adopted %+v", got)
	}
	if got := lineOf(t, dry, reloaded); got.Result != ResCreate || got.Attention != "" || !strings.Contains(got.Detail, "no files yet") {
		t.Fatalf("wanted %+v", got)
	}
}
