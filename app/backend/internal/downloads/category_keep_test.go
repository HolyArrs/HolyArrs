package downloads

import (
	"context"
	"database/sql"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/doctor"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A later read that works must not wipe the missing-category line a check
// stored; only a check that finds the category clears it.
func TestBackgroundReadKeepsMissingCategory(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	id := w.addDownloader(t, &domain.Downloader{Name: "Sab", Adapter: "sabnzbd", Protocol: "usenet", URL: "http://sab", Priority: 5})
	m.noteCategory(ctx, id, ErrNoCategory)
	line := w.downloader(t, id).LastError
	if doctor.OfText(line) != doctor.Category {
		t.Fatalf("stored %q", line)
	}
	m.downloaderOK(ctx, w.downloader(t, id))
	if got := w.downloader(t, id).LastError; got != line {
		t.Fatalf("a background read wiped the category line: %q", got)
	}
	// After a failure the back-off still ends, and the line stays.
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE downloaders SET fail_count = 2 WHERE id = ?", id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	m.downloaderOK(ctx, w.downloader(t, id))
	if d := w.downloader(t, id); d.FailCount != 0 || d.LastError != line {
		t.Fatalf("after recovery %+v", d)
	}
	m.categoryFound(ctx, w.downloader(t, id))
	if got := w.downloader(t, id).LastError; got != "" {
		t.Fatalf("a check that found the category kept %q", got)
	}
}
