package subtitles

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/sites/fakesite"
)

// A title that left the Library keeps its file rows, but its files are no
// longer looked at, listed as missing or searched for.
func TestRemovedTitleWantsNothing(t *testing.T) {
	f := newFix(t)
	s, _ := svc(t, f, &fakesite.Site{})
	ctx := context.Background()
	wishAll(t, f, Want{Language: "es"})
	if _, err := s.Look(ctx, f.movieFile, domain.System); err != nil {
		t.Fatal(err)
	}
	if _, n, _ := MissingList(ctx, f.st.Read(), MissingFilter{TitleID: f.movie}, 50, 0); n != 1 {
		t.Fatalf("before: %d missing for the movie", n)
	}
	if err := f.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE titles SET state = 'removed' WHERE id = ?", f.movie)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, n, _ := MissingList(ctx, f.st.Read(), MissingFilter{TitleID: f.movie}, 50, 0); n != 0 {
		t.Fatalf("a removed title is listed: %d missing", n)
	}
	titles, err := MissingTitles(ctx, f.st.Read())
	if err != nil || slices.ContainsFunc(titles, func(m MissingTitle) bool { return m.ID == f.movie }) {
		t.Fatalf("missing titles %+v %v", titles, err)
	}
	due, err := s.dueFiles(ctx, 100)
	if err != nil || slices.Contains(due, f.movieFile) {
		t.Fatalf("due files %v %v", due, err)
	}
	stale, err := s.staleFiles(ctx, 100, nil, map[int64]bool{})
	if err != nil || slices.Contains(stale, f.movieFile) || !slices.Contains(stale, f.ep1File) {
		t.Fatalf("files to look at %v %v", stale, err)
	}
	if ids, err := FilesOf(ctx, f.st.Read(), f.movie, 0); err != nil || len(ids) != 0 {
		t.Fatalf("files of a removed title %v %v", ids, err)
	}
	if _, err := s.Look(ctx, f.movieFile, domain.System); !errors.Is(err, ErrNotFound) {
		t.Fatalf("look: %v", err)
	}
	if _, err := s.Search(ctx, f.movieFile, SearchOptions{}, domain.System); !errors.Is(err, ErrNotFound) {
		t.Fatalf("search: %v", err)
	}
}
