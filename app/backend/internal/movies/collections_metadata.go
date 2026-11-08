package movies

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// syncMovieCollectionTx records confirmed TMDB movie-series membership. Search
// results do not know membership, so they preserve the last confirmed value.
// The caller owns the transaction and the Store file lock.
func syncMovieCollectionTx(ctx context.Context, tx *sql.Tx, titleID int64, movie metadata.Movie) error {
	if !movie.CollectionKnown {
		return nil
	}
	var sourceID, name string
	if movie.Collection != nil {
		id := strings.TrimSpace(movie.Collection.ID)
		name = strings.TrimSpace(movie.Collection.Name)
		n, err := strconv.ParseInt(id, 10, 64)
		if err != nil || n <= 0 || name == "" {
			return errors.New("invalid TMDB movie collection")
		}
		sourceID = "tmdb:" + strconv.FormatInt(n, 10)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM movie_collection_titles
		WHERE title_id = ? AND collection_id IN (
			SELECT id FROM movie_collections WHERE substr(source_id, 1, 5) = 'tmdb:'
		)`, titleID); err != nil {
		return err
	}
	if sourceID == "" {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO movie_collections(source_id, name) VALUES(?, ?)
		ON CONFLICT(source_id) DO UPDATE SET name = excluded.name`, sourceID, name); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO movie_collection_titles(collection_id, title_id)
		SELECT id, ? FROM movie_collections WHERE source_id = ?
		ON CONFLICT(collection_id, title_id) DO NOTHING`, titleID, sourceID)
	return err
}
