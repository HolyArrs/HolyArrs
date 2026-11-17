package metadata

import (
	"context"
	"testing"
)

// A TVMaze search result carries tvdb/imdb ids but no tmdb id; the
// tmdb id is found through TMDB's find without loading the whole show.
func TestTMDBShowIDFromOtherIDs(t *testing.T) {
	tm := newTMDB(fakeTMDBTV(t), fakeCred)
	for _, id := range []string{"tvdb:200", "imdb:tt0100"} {
		got, err := tm.ShowTMDBID(context.Background(), id)
		if err != nil || got != "100" {
			t.Fatalf("%s: got %q, %v", id, got, err)
		}
	}
	if _, err := tm.ShowTMDBID(context.Background(), "tvmaze:5"); err == nil {
		t.Fatal("tvmaze id should not be found through TMDB")
	}
}
