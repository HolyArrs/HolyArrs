package sources

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

// TestRemove_SourceWithManyReleases checks that a Source that has gathered far
// more Releases and calls than one short write can delete is still removed.
func TestRemove_SourceWithManyReleases(t *testing.T) {
	ctx := context.Background()
	s, err := database.Open(t.TempDir(), database.Options{WriteCap: 250 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	v, err := secrets.Open(ctx, s, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	src, err := Add(ctx, s, v, Input{Name: "Big", Format: "rss", URL: "https://example.test/rss"}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	const n = 300000
	err = s.Write(database.WithLongWrite(ctx), func(tx *sql.Tx) error {
		now := database.FormatTime(time.Now())
		if _, err := tx.Exec(`WITH RECURSIVE k(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM k WHERE i < ?)
			INSERT INTO releases (created_at, created_by, updated_at, updated_by, source_id, name, link_ref, protocol, found_by, state)
			SELECT ?, 'system', ?, 'system', ?, 'Some.Show.S01E' || i || '.1080p', 'l' || i, 'torrent', 'feed', 'passed_over' FROM k`, n, now, now, src.ID); err != nil {
			return err
		}
		_, err := tx.Exec(`WITH RECURSIVE k(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM k WHERE i < ?)
			INSERT INTO source_calls(source_id, at, op, ok, ms, error_kind) SELECT ?, ?, 'search', 1, 5, '' FROM k`, n, src.ID, now)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := Remove(ctx, s, src.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	for _, q := range []string{"SELECT count(*) FROM sources WHERE id = ?", "SELECT count(*) FROM releases WHERE source_id = ?", "SELECT count(*) FROM source_calls WHERE source_id = ?"} {
		var left int
		if err := s.Read().QueryRowContext(ctx, q, src.ID).Scan(&left); err != nil || left != 0 {
			t.Fatalf("%s: %d left (%v)", q, left, err)
		}
	}
}
