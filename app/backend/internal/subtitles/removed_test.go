package subtitles

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A title removed from the Library gets no subtitle work: no look, no
// Missing row, no due search, and its pending wishes are cleaned.
func TestRemovedTitle_GetsNoSubtitleWork(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	s := &Service{Store: f.st}
	wishAll(t, f, Want{Language: "es"})
	if _, err := s.Look(ctx, f.movieFile, domain.System); err != nil {
		t.Fatal(err)
	}
	if err := f.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE titles SET state = 'removed' WHERE id = ?", f.movie)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Look(ctx, f.movieFile, domain.System); !errors.Is(err, ErrNotMain) {
		t.Fatalf("look of a removed title: %v", err)
	}
	rows, _, err := MissingList(ctx, f.st.Read(), MissingFilter{}, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.TitleID == f.movie {
			t.Fatal("removed title listed as missing")
		}
	}
	if ids, _ := FilesOf(ctx, f.st.Read(), f.movie, 0); len(ids) != 0 {
		t.Fatalf("files of removed title %v", ids)
	}
	if err := s.dropRemoved(ctx); err != nil {
		t.Fatal(err)
	}
	ids, err := s.dueFiles(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if id == f.movieFile {
			t.Fatal("removed title still due for a search")
		}
	}
	var n int
	_ = f.st.Read().QueryRow("SELECT count(*) FROM subtitles WHERE file_id = ? AND place = 'wish'", f.movieFile).Scan(&n)
	if n != 0 {
		t.Fatalf("pending wishes left %d", n)
	}
}

// The Missing titles list, the files-to-look-at pass and a direct search also
// leave a removed title out, while its other titles stay.
func TestRemovedTitle_LeftOutOfTitlesStaleAndSearch(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	s := &Service{Store: f.st}
	wishAll(t, f, Want{Language: "es"})
	if _, err := s.Look(ctx, f.movieFile, domain.System); err != nil {
		t.Fatal(err)
	}
	if err := f.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE titles SET state = 'removed' WHERE id = ?", f.movie)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	titles, err := MissingTitles(ctx, f.st.Read())
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range titles {
		if m.ID == f.movie {
			t.Fatal("removed title in the Missing titles list")
		}
	}
	stale, err := s.staleFiles(ctx, 100, nil, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	seenEp := false
	for _, id := range stale {
		if id == f.movieFile {
			t.Fatal("removed title still looked at")
		}
		seenEp = seenEp || id == f.ep1File
	}
	if !seenEp {
		t.Fatalf("the show's file should still be looked at: %v", stale)
	}
	if _, err := s.Search(ctx, f.movieFile, SearchOptions{}, domain.System); err == nil {
		t.Fatal("search of a removed title went ahead")
	}
}
