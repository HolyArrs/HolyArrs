package downloads

import (
	"context"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// The page's own rows are the newest 100 live downloads. Cleaned rows are
// not on the page, so they must not take a place in that window and make a
// live own row look like an outside one in a live patch.
func TestOwnRowsUsesTheViewsLiveWindow(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotDownloading, "downloading")
	var did int64
	if err := x.s.Read().QueryRow("SELECT downloader_id FROM downloads WHERE id = ?", x.dlID).Scan(&did); err != nil {
		t.Fatal(err)
	}
	// 100 newer cleaned rows fill an unfiltered newest-100 window.
	exec41(t, x.s, `WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 100)
		INSERT INTO downloads (id, created_at, created_by, updated_at, updated_by, downloader_id, external_id, state, started_by)
		SELECT 1000 + i, 't', '{}', 't', '{}', ?, 'dead' || printf('%03d', i), 'cleaned', '{}' FROM n`, did)
	var ext string
	if err := x.s.Read().QueryRow("SELECT lower(external_id) FROM downloads WHERE id = ?", x.dlID).Scan(&ext); err != nil {
		t.Fatal(err)
	}
	key := LiveKey(did, ext)
	own := x.dl.ownRows(context.Background(), did, []string{key}, false)
	if !own.keys[key] {
		t.Fatalf("the live row is on the page, but ownRows left it out: %+v", own.keys)
	}
}
