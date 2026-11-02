package api

import (
	"database/sql"
	"errors"
	"time"

	_ "modernc.org/sqlite"
)

// TileStore keeps each genre's chosen slideshow photos across restarts:
// the artist, the source picture link, when it was picked, and the rendered
// JPEG, so a restart or a dropped in-memory genre never leaves it without
// art. The table is add-only (CREATE TABLE IF NOT EXISTS). The older
// genre_tiles table (one picture per genre, some of them covers) is left
// in place and no longer read.
type TileStore struct{ db *sql.DB }

type storedTile struct {
	Slug, Artist, MBID, ImageURL, ETag string
	Idx                                int
	JPEG                               []byte
	PickedAt                           time.Time
}

// OpenTileStore opens (and if needed creates) the tile store at path.
func OpenTileStore(path string) (*TileStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;
CREATE TABLE IF NOT EXISTS genre_photos (slug TEXT NOT NULL, idx INTEGER NOT NULL, artist TEXT NOT NULL, artist_mbid TEXT NOT NULL, image_url TEXT NOT NULL, jpeg BLOB NOT NULL, etag TEXT NOT NULL, picked_at INTEGER NOT NULL, PRIMARY KEY(slug, idx)) WITHOUT ROWID;`); err != nil {
		db.Close()
		return nil, err
	}
	return &TileStore{db: db}, nil
}

func (s *TileStore) Close() error { return s.db.Close() }

// PutSet replaces a genre's photos; an empty photo or one with no source
// link is refused, so the store never holds an empty or unlinked entry.
func (s *TileStore) PutSet(slug string, ts []storedTile) error {
	for _, t := range ts {
		if t.Slug != slug || t.ImageURL == "" || len(t.JPEG) == 0 {
			return errors.New("tile store: empty tile")
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM genre_photos WHERE slug=?`, slug); err != nil {
		return err
	}
	for _, t := range ts {
		if _, err := tx.Exec(`INSERT INTO genre_photos(slug,idx,artist,artist_mbid,image_url,jpeg,etag,picked_at) VALUES(?,?,?,?,?,?,?,?)`,
			t.Slug, t.Idx, t.Artist, t.MBID, t.ImageURL, t.JPEG, t.ETag, t.PickedAt.Unix()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Get reads one genre's photos in order.
func (s *TileStore) Get(slug string) []storedTile {
	rows, err := s.db.Query(`SELECT slug,idx,artist,artist_mbid,image_url,jpeg,etag,picked_at FROM genre_photos WHERE slug=? ORDER BY idx`, slug)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []storedTile
	for rows.Next() {
		var t storedTile
		var at int64
		if rows.Scan(&t.Slug, &t.Idx, &t.Artist, &t.MBID, &t.ImageURL, &t.JPEG, &t.ETag, &at) == nil {
			t.PickedAt = time.Unix(at, 0)
			out = append(out, t)
		}
	}
	return out
}

// Slugs lists up to n stored genres, newest first.
func (s *TileStore) Slugs(n int) []string {
	rows, err := s.db.Query(`SELECT slug FROM genre_photos GROUP BY slug ORDER BY MAX(picked_at) DESC LIMIT ?`, n)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if rows.Scan(&v) == nil {
			out = append(out, v)
		}
	}
	return out
}
