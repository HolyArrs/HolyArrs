package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// route: GET /api/v1/extras/{kind}/{id}
// A Library movie's kept OMDb scores fill the sources the outside ratings
// have none for; a title outside the Library gets none.
func TestTitleExtrasAddKeptOMDbScores(t *testing.T) {
	t.Parallel()
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	err = st.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO titles (created_at, created_by, updated_at, updated_by, kind, name, sort_name, source_ids, metadata_source, why_added, state,
			imdb_rating, imdb_votes, rotten_tomatoes, metacritic) VALUES ('2026-01-01T00:00:00Z', '{}', '2026-01-01T00:00:00Z', '{}', 'movie', 'Dune', 'dune',
			'{"tmdb":"438631","imdb":"tt1160419"}', 'tmdb', 'by_hand', 'active', 7.9, 950000, 83, 74)`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(buildHandlers(services{Store: st, Version: "test",
		ExtraRatings: func(_ context.Context, kind, id string) (metadata.ServerRatings, error) {
			return metadata.ServerRatings{Scores: []metadata.ServerScore{{Source: "imdb", Value: 8.0, Max: 10, Votes: 960000}}}, nil
		}}).Handler())
	defer srv.Close()
	var v metadata.TitleExtras
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/extras/movie/438631", "", 200), &v)
	got := map[string]float64{}
	for _, s := range v.Scores {
		got[s.Source] = s.Value
	}
	if len(v.Scores) != 3 || got["imdb"] != 8.0 || got["rt_critics"] != 83 || got["metacritic"] != 74 {
		t.Fatalf("scores %+v", v.Scores)
	}
	v = metadata.TitleExtras{}
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/extras/movie/1", "", 200), &v)
	if len(v.Scores) != 1 {
		t.Fatalf("not in the Library: %+v", v.Scores)
	}
}
