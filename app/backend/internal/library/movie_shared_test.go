package library

import (
	"context"
	"database/sql"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// TestSyncMovieCollectionHook: with no hook set, recording a collection is a
// quiet no-op; once UseMovies sets one, it runs with the Title's id.
func TestSyncMovieCollectionHook(t *testing.T) {
	old := movieCollectionSync
	t.Cleanup(func() { UseMovies(old) })

	UseMovies(nil)
	if err := syncMovieCollection(context.Background(), nil, 1, metadata.Movie{}); err != nil {
		t.Fatalf("nil hook: %v", err)
	}

	var got int64
	calls := 0
	UseMovies(func(_ context.Context, _ *sql.Tx, titleID int64, _ metadata.Movie) error {
		calls++
		got = titleID
		return nil
	})
	if err := syncMovieCollection(context.Background(), nil, 42, metadata.Movie{}); err != nil {
		t.Fatalf("hook: %v", err)
	}
	if calls != 1 || got != 42 {
		t.Fatalf("hook calls=%d titleID=%d, want 1 and 42", calls, got)
	}
}
