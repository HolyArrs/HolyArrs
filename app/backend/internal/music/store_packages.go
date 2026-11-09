package music

import (
	"context"
	"database/sql"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// packageRow is one stored package as the copy writes it.
type packageRow struct {
	mbid, hash string
	version    int64
	rank       int
	complete   bool
	body       []byte
	pictures   string
}

// packageAlbum is one album a stored package lists.
type packageAlbum struct{ rg, cover string }

// packageIndexRow is one entry of the artist index read at start.
type packageIndexRow struct {
	mbid, pictures string
	rank           int
}

// readPackageIndex reads every stored package's key, rank and pictures.
func readPackageIndex(ctx context.Context, db *sql.DB) ([]packageIndexRow, error) {
	rows, err := db.QueryContext(ctx, "SELECT mbid, rank, pictures FROM music_packages")
	if err != nil {
		return nil, err
	}
	var out []packageIndexRow
	for rows.Next() {
		var r packageIndexRow
		if err := rows.Scan(&r.mbid, &r.rank, &r.pictures); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, r)
	}
	rows.Close()
	return out, rows.Err()
}

// readPackageSync reads the saved cursor and last copy time. Missing reads as zero.
func readPackageSync(ctx context.Context, q database.Querier) (cursor int64, at string) {
	_ = q.QueryRowContext(ctx, "SELECT cursor, synced_at FROM music_package_sync WHERE id = 1").Scan(&cursor, &at)
	return cursor, at
}

// readPackageCursor reads the saved cursor. Missing reads as zero.
func readPackageCursor(ctx context.Context, q database.Querier) (cursor int64) {
	_ = q.QueryRowContext(ctx, "SELECT cursor FROM music_package_sync WHERE id = 1").Scan(&cursor)
	return cursor
}

// readPackageBytes reads the stored size of every package body.
func readPackageBytes(ctx context.Context, q database.Querier) (size int64) {
	_ = q.QueryRowContext(ctx, "SELECT coalesce(sum(length(body)), 0) FROM music_packages").Scan(&size)
	return size
}

// readPackageBody reads one package's packed body.
func readPackageBody(ctx context.Context, q database.Querier, mbid string) (body []byte, err error) {
	err = q.QueryRowContext(ctx, "SELECT body FROM music_packages WHERE mbid = ?", mbid).Scan(&body)
	return body, err
}

// readPackageAlbum reads one listed album's artist and cover record.
func readPackageAlbum(ctx context.Context, q database.Querier, rg string) (artist, cover string, err error) {
	err = q.QueryRowContext(ctx, "SELECT artist, cover FROM music_package_albums WHERE rg = ?", rg).Scan(&artist, &cover)
	return artist, cover, err
}

// countPackageAlbums counts the albums the packages list.
func countPackageAlbums(ctx context.Context, q database.Querier) (n int) {
	_ = q.QueryRowContext(ctx, "SELECT count(*) FROM music_package_albums").Scan(&n)
	return n
}

// putPackageTx stores one package and replaces the albums it lists.
func putPackageTx(tx *sql.Tx, p packageRow, albums []packageAlbum, now string) error {
	if _, err := tx.Exec(`INSERT INTO music_packages (mbid, version, hash, rank, complete, body, pictures, stored_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT (mbid) DO UPDATE SET version = excluded.version, hash = excluded.hash, rank = excluded.rank, complete = excluded.complete,
				body = excluded.body, pictures = excluded.pictures, stored_at = excluded.stored_at`,
		p.mbid, p.version, p.hash, p.rank, p.complete, p.body, p.pictures, now); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM music_package_albums WHERE artist = ?", p.mbid); err != nil {
		return err
	}
	for _, a := range albums {
		if _, err := tx.Exec(`INSERT INTO music_package_albums (rg, artist, cover) VALUES (?, ?, ?)
					ON CONFLICT (rg) DO UPDATE SET artist = excluded.artist, cover = excluded.cover`, a.rg, p.mbid, a.cover); err != nil {
			return err
		}
	}
	return nil
}

// putPackageCursorTx saves the cursor and the copy time.
func putPackageCursorTx(tx *sql.Tx, cursor int64, now string) error {
	_, err := tx.Exec(`INSERT INTO music_package_sync (id, cursor, synced_at) VALUES (1, ?, ?)
			ON CONFLICT (id) DO UPDATE SET cursor = excluded.cursor, synced_at = excluded.synced_at`, cursor, now)
	return err
}
