package mediafiles

import (
	"context"
	"database/sql"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// OutsideScores are a title's IMDb, Rotten Tomatoes and Metacritic scores
// from OMDb; nil fields are not known.
type OutsideScores struct {
	IMDbRating     *float64
	IMDbVotes      int
	RottenTomatoes *int
	Metacritic     *int
}

// Empty says no score is known.
func (s OutsideScores) Empty() bool {
	return s.IMDbRating == nil && s.RottenTomatoes == nil && s.Metacritic == nil
}

// OMDbDue is a title whose scores are due from OMDb, with its IMDb id.
type OMDbDue struct {
	ID   int64
	IMDb string
}

// TitlesDueForOMDb lists up to limit movies and shows with an IMDb id whose
// scores were never read, or last read before before: never read first,
// then the oldest.
func TitlesDueForOMDb(ctx context.Context, db database.Lister, before time.Time, limit int) ([]OMDbDue, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, json_extract(source_ids, '$.imdb') FROM titles
		WHERE kind IN ('movie', 'show') AND state != 'removed' AND dropped_at IS NULL
		AND coalesce(json_extract(source_ids, '$.imdb'), '') != ''
		AND (omdb_checked_at IS NULL OR omdb_checked_at < ?)
		ORDER BY omdb_checked_at IS NOT NULL, omdb_checked_at, id LIMIT ?`, database.FormatTime(before), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OMDbDue
	for rows.Next() {
		var d OMDbDue
		if err := rows.Scan(&d.ID, &d.IMDb); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// SaveOMDbScores records that OMDb was asked about a title at at. found
// false (OMDb does not know it) keeps the scores already kept. The title's
// updated_at moves only when a score changed.
func SaveOMDbScores(ctx context.Context, s *database.Store, titleID int64, sc OutsideScores, found bool, at time.Time) error {
	now := database.FormatTime(at)
	return s.Write(ctx, func(tx *sql.Tx) error {
		if found {
			_, err := tx.Exec(`UPDATE titles SET imdb_rating = ?, imdb_votes = ?, rotten_tomatoes = ?, metacritic = ?, updated_at = ?
				WHERE id = ? AND (imdb_rating IS NOT ? OR imdb_votes != ? OR rotten_tomatoes IS NOT ? OR metacritic IS NOT ?)`,
				sc.IMDbRating, sc.IMDbVotes, sc.RottenTomatoes, sc.Metacritic, now,
				titleID, sc.IMDbRating, sc.IMDbVotes, sc.RottenTomatoes, sc.Metacritic)
			if err != nil {
				return err
			}
		}
		_, err := tx.Exec("UPDATE titles SET omdb_checked_at = ? WHERE id = ?", now, titleID)
		return err
	})
}

// OMDbScoresByTMDB reads the kept scores of the Library's movie or show with
// that TMDB id; ok is false when it is not in the Library or has none.
func OMDbScoresByTMDB(ctx context.Context, db database.Lister, kind, tmdbID string) (OutsideScores, bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT imdb_rating, imdb_votes, rotten_tomatoes, metacritic FROM titles
		WHERE kind = ? AND state != 'removed' AND json_extract(source_ids, '$.tmdb') = ?
		AND (imdb_rating IS NOT NULL OR rotten_tomatoes IS NOT NULL OR metacritic IS NOT NULL) ORDER BY id LIMIT 1`, kind, tmdbID)
	if err != nil {
		return OutsideScores{}, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return OutsideScores{}, false, rows.Err()
	}
	var sc OutsideScores
	var imdb sql.NullFloat64
	var rt, mc sql.NullInt64
	if err := rows.Scan(&imdb, &sc.IMDbVotes, &rt, &mc); err != nil {
		return OutsideScores{}, false, err
	}
	if imdb.Valid {
		sc.IMDbRating = &imdb.Float64
	}
	if rt.Valid {
		v := int(rt.Int64)
		sc.RottenTomatoes = &v
	}
	if mc.Valid {
		v := int(mc.Int64)
		sc.Metacritic = &v
	}
	return sc, !sc.Empty(), rows.Err()
}
