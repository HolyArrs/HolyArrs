package movies

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func syncCollection(t *testing.T, s *database.Store, titleID int64, movie metadata.Movie) error {
	t.Helper()
	return s.Write(context.Background(), func(tx *sql.Tx) error {
		return syncMovieCollectionTx(context.Background(), tx, titleID, movie)
	})
}

func collectionMemberships(t *testing.T, s *database.Store, titleID int64) map[string]string {
	t.Helper()
	rows, err := s.Read().Query(`SELECT c.source_id, c.name FROM movie_collections c
		JOIN movie_collection_titles ct ON ct.collection_id = c.id
		WHERE ct.title_id = ? ORDER BY c.source_id`, titleID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]string{}
	for rows.Next() {
		var sourceID, name string
		if err := rows.Scan(&sourceID, &name); err != nil {
			t.Fatal(err)
		}
		got[sourceID] = name
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestSyncMovieCollectionKnownMembershipAndRename(t *testing.T) {
	_, s, _, titleID := renameWorld(t)
	movie := metadata.Movie{CollectionKnown: true, Collection: &metadata.MovieCollection{ID: "726871", Name: "Dune Collection"}}
	if err := syncCollection(t, s, titleID, movie); err != nil {
		t.Fatal(err)
	}
	movie.Collection.Name = "Dune Saga"
	if err := syncCollection(t, s, titleID, movie); err != nil {
		t.Fatal(err)
	}
	got := collectionMemberships(t, s, titleID)
	if len(got) != 1 || got["tmdb:726871"] != "Dune Saga" {
		t.Fatalf("memberships %v", got)
	}
}

func TestSyncMovieCollectionKnownNonePreservesOtherSources(t *testing.T) {
	_, s, _, titleID := renameWorld(t)
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		for _, row := range [][2]string{{"tmdb:1", "Old"}, {"local:family", "Family"}} {
			if _, err := tx.Exec(`INSERT INTO movie_collections(source_id, name) VALUES(?, ?)`, row[0], row[1]); err != nil {
				return err
			}
			if _, err := tx.Exec(`INSERT INTO movie_collection_titles(collection_id, title_id)
				SELECT id, ? FROM movie_collections WHERE source_id = ?`, titleID, row[0]); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := syncCollection(t, s, titleID, metadata.Movie{CollectionKnown: true}); err != nil {
		t.Fatal(err)
	}
	got := collectionMemberships(t, s, titleID)
	if len(got) != 1 || got["local:family"] != "Family" {
		t.Fatalf("memberships %v", got)
	}
}

func TestSyncMovieCollectionUnknownAndInvalidPreservePrior(t *testing.T) {
	_, s, _, titleID := renameWorld(t)
	known := metadata.Movie{CollectionKnown: true, Collection: &metadata.MovieCollection{ID: "7", Name: "Seven"}}
	if err := syncCollection(t, s, titleID, known); err != nil {
		t.Fatal(err)
	}
	if err := syncCollection(t, s, titleID, metadata.Movie{}); err != nil {
		t.Fatal(err)
	}
	bad := metadata.Movie{CollectionKnown: true, Collection: &metadata.MovieCollection{ID: "bad", Name: "Broken"}}
	if err := syncCollection(t, s, titleID, bad); err == nil {
		t.Fatal("invalid collection accepted")
	}
	got := collectionMemberships(t, s, titleID)
	if len(got) != 1 || got["tmdb:7"] != "Seven" {
		t.Fatalf("memberships %v", got)
	}
}

func TestAddAndRefreshSyncConfirmedMovieCollection(t *testing.T) {
	s := open(t)
	addFolder(t, s)
	ctx := context.Background()
	initial := dune
	initial.CollectionKnown = true
	initial.Collection = &metadata.MovieCollection{ID: "726871", Name: "Dune Collection"}
	titleID, err := library.AddMovie(ctx, s, initial, library.AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if got := collectionMemberships(t, s, titleID); got["tmdb:726871"] != "Dune Collection" {
		t.Fatalf("after add %v", got)
	}
	updated := initial
	updated.Collection = nil
	svc := New(&library.Service{Store: s, Metadata: &metadata.Static{Movies: []metadata.Movie{updated}}, Now: func() time.Time { return now }})
	if _, err := svc.RefreshTitle(ctx, titleID, domain.System, nil); err != nil {
		t.Fatal(err)
	}
	if got := collectionMemberships(t, s, titleID); len(got) != 0 {
		t.Fatalf("after refresh %v", got)
	}
}
