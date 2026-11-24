package downloads

import (
	"context"
	"database/sql"
	"testing"
)

// A deleted title's links go: its client link row is dropped and its match
// row goes back to the matcher; another title's link stays.
func TestUnlinkTitleTx(t *testing.T) {
	ctx := context.Background()
	s, _ := openSettingsWorld(t)
	exec := func(q string, args ...any) {
		t.Helper()
		if err := s.Write(ctx, func(tx *sql.Tx) error { _, err := tx.Exec(q, args...); return err }); err != nil {
			t.Fatal(err)
		}
	}
	for _, r := range []struct {
		hash  string
		title int64
	}{{"aaa", 7}, {"bbb", 8}} {
		exec(`INSERT INTO client_links (downloader_id, hash, state, title_id, how, matched_at) VALUES (1, ?, 'linked', ?, 'name', '2026-11-21T00:00:00Z')`, r.hash, r.title)
		exec(`INSERT INTO dl_match_queue (downloader_id, key, outcome, attempts, next_try_at, linked_title_id) VALUES (1, ?, 'linked', 1, '2026-11-21T00:00:00Z', ?)`, r.hash, r.title)
	}
	if err := s.Write(ctx, func(tx *sql.Tx) error { return UnlinkTitleTx(tx, 7) }); err != nil {
		t.Fatal(err)
	}
	var links int
	_ = s.Read().QueryRow(`SELECT count(*) FROM client_links WHERE title_id = 7`).Scan(&links)
	var outcome, next string
	var linked sql.NullInt64
	var tries int
	_ = s.Read().QueryRow(`SELECT outcome, linked_title_id, attempts, next_try_at FROM dl_match_queue WHERE key = 'aaa'`).Scan(&outcome, &linked, &tries, &next)
	if links != 0 || outcome != "" || linked.Valid || tries != 0 || next != "" {
		t.Fatalf("links %d, queue %q %v %d %q", links, outcome, linked, tries, next)
	}
	var other int
	_ = s.Read().QueryRow(`SELECT count(*) FROM client_links l JOIN dl_match_queue q ON q.key = l.hash WHERE l.title_id = 8 AND q.linked_title_id = 8`).Scan(&other)
	if other != 1 {
		t.Fatalf("other title's link: %d", other)
	}
}
