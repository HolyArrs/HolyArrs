package mediafiles

import (
	"context"
	"database/sql"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// TraktDue is a movie or show whose Trakt rating is due, with its kind and
// IMDb id (Trakt finds a title by it).
type TraktDue struct {
	ID   int64
	Kind string
	IMDb string
}

// TitlesDueForTrakt lists up to limit movies and shows with an IMDb id whose
// Trakt rating was never read, or last read before before: never read
// first, then the oldest.
func TitlesDueForTrakt(ctx context.Context, db database.Lister, before time.Time, limit int) ([]TraktDue, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, kind, json_extract(source_ids, '$.imdb') FROM titles
		WHERE kind IN ('movie', 'show') AND state != 'removed' AND dropped_at IS NULL
		AND coalesce(json_extract(source_ids, '$.imdb'), '') != ''
		AND (trakt_checked_at IS NULL OR trakt_checked_at < ?)
		ORDER BY trakt_checked_at IS NOT NULL, trakt_checked_at, id LIMIT ?`, database.FormatTime(before), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TraktDue
	for rows.Next() {
		var d TraktDue
		if err := rows.Scan(&d.ID, &d.Kind, &d.IMDb); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// SaveTraktRating records that Trakt was asked about a title at at. found
// false (Trakt does not know it) keeps the rating already kept; a title
// with no votes keeps no rating. The title's updated_at moves only when
// the rating changed.
func SaveTraktRating(ctx context.Context, s *database.Store, titleID int64, rating float64, votes int, found bool, at time.Time) error {
	now := database.FormatTime(at)
	return s.Write(ctx, func(tx *sql.Tx) error {
		if found {
			var r any
			if votes > 0 {
				r = rating
			}
			_, err := tx.Exec(`UPDATE titles SET trakt_rating = ?, trakt_votes = ?, updated_at = ?
				WHERE id = ? AND (trakt_rating IS NOT ? OR trakt_votes != ?)`, r, votes, now, titleID, r, votes)
			if err != nil {
				return err
			}
		}
		_, err := tx.Exec("UPDATE titles SET trakt_checked_at = ? WHERE id = ?", now, titleID)
		return err
	})
}

// TraktRatingByTMDB reads the kept Trakt rating of the Library's movie or
// show with that TMDB id; ok is false when it is not in the Library or has
// none.
func TraktRatingByTMDB(ctx context.Context, db database.Lister, kind, tmdbID string) (float64, int, bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT trakt_rating, trakt_votes FROM titles
		WHERE kind = ? AND state != 'removed' AND json_extract(source_ids, '$.tmdb') = ? AND trakt_rating IS NOT NULL
		ORDER BY id LIMIT 1`, kind, tmdbID)
	if err != nil {
		return 0, 0, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return 0, 0, false, rows.Err()
	}
	var rating float64
	var votes int
	if err := rows.Scan(&rating, &votes); err != nil {
		return 0, 0, false, err
	}
	return rating, votes, true, rows.Err()
}
