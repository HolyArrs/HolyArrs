package library

import (
	"context"
	"database/sql"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// MovieCollectionSync records a movie's collection inside the add or
// refresh transaction. The movies package sets it with UseMovies.
type MovieCollectionSync func(ctx context.Context, tx *sql.Tx, titleID int64, movie metadata.Movie) error

var movieCollectionSync MovieCollectionSync

// UseMovies sets the collection step that runs when a movie is added or
// refreshed. Without it a movie's collection is not recorded.
func UseMovies(sync MovieCollectionSync) { movieCollectionSync = sync }

func syncMovieCollection(ctx context.Context, tx *sql.Tx, titleID int64, movie metadata.Movie) error {
	if movieCollectionSync == nil {
		return nil
	}
	return movieCollectionSync(ctx, tx, titleID, movie)
}
