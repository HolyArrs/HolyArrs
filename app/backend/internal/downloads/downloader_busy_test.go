package downloads

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// While a Download is running, pointing the Downloader at another host or
// category is refused; a rename is fine.
func TestChangeRefusesHostOrCategoryWhileActive(t *testing.T) {
	ctx := context.Background()
	s, v := openSettingsWorld(t)
	d, err := Add(ctx, s, v, Input{Name: "q", Adapter: "qbittorrent", URL: "http://10.0.0.5:8080", Secret: "k"}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		dl := &domain.Download{DownloaderID: d.ID, SlotIDs: []int64{}, ExternalID: "abc", State: "downloading", FilesSeen: []domain.SeenFile{}, StartedBy: domain.System}
		_, err := database.Insert(tx, "downloads", dl, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	cat := "other"
	if _, err := Change(ctx, s, v, d.ID, Input{Name: "q", Adapter: "qbittorrent", URL: d.URL, Category: &cat}, domain.System); !errors.Is(err, ErrBusy) {
		t.Fatalf("category: %v", err)
	}
	if _, err := Change(ctx, s, v, d.ID, Input{Name: "q", Adapter: "qbittorrent", URL: "http://10.0.0.6:8080", Secret: "k"}, domain.System); !errors.Is(err, ErrBusy) {
		t.Fatalf("host: %v", err)
	}
	if _, err := Change(ctx, s, v, d.ID, Input{Name: "q2", Adapter: "qbittorrent", URL: d.URL}, domain.System); err != nil {
		t.Fatalf("rename: %v", err)
	}
}
