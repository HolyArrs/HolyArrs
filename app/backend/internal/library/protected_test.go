package library

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestConfirmDownloadEdition(t *testing.T) {
	svc, s, _, id := renameWorld(t)
	ctx := context.Background()
	slot := slotsOf(t, s, id)[0].ID
	add := func(ext, state string) int64 {
		var dl int64
		if err := s.Write(ctx, func(tx *sql.Tx) error {
			var n int
			_ = tx.QueryRow("SELECT COUNT(*) FROM downloaders").Scan(&n)
			if n == 0 {
				if _, err := tx.Exec(`INSERT INTO downloaders (created_at, created_by, updated_at, updated_by, name, adapter, protocol, url) VALUES ('2026-01-01T00:00:00Z', '{}', '2026-01-01T00:00:00Z', '{}', 'qb', 'qbittorrent', 'torrent', 'http://x')`); err != nil {
					return err
				}
			}
			r, err := tx.Exec(`INSERT INTO downloads (created_at, created_by, updated_at, updated_by, downloader_id, slot_ids, external_id, state, started_by)
				VALUES ('2026-01-01T00:00:00Z', '{}', '2026-01-01T00:00:00Z', '{}', (SELECT MIN(id) FROM downloaders), ?, ?, ?, '{}')`, "["+strconv.FormatInt(slot, 10)+"]", ext, state)
			if err != nil {
				return err
			}
			dl, err = r.LastInsertId()
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return dl
	}
	done := add("a", "finished")
	if err := svc.ConfirmDownloadEdition(ctx, done, "Director's Cut", domain.System); err != nil {
		t.Fatal(err)
	}
	var d domain.Download
	if err := database.Get(ctx, s.Read(), "downloads", done, &d); err != nil {
		t.Fatal(err)
	}
	if d.ConfirmedEdition == "" {
		t.Fatalf("edition not stored: %+v", d)
	}
	if err := svc.ConfirmDownloadEdition(ctx, done, "", domain.System); !errors.Is(err, ErrCopyChoice) {
		t.Fatalf("empty edition: %v", err)
	}
	if err := svc.ConfirmDownloadEdition(ctx, 99999, "Director's Cut", domain.System); err == nil {
		t.Fatal("unknown download accepted")
	}
	busy := add("b", "downloading")
	if err := svc.ConfirmDownloadEdition(ctx, busy, "Director's Cut", domain.System); !errors.Is(err, ErrCopyChoice) {
		t.Fatalf("unfinished download: %v", err)
	}
}
