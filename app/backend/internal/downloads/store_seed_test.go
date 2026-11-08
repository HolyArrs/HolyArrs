package downloads

import (
	"context"
	"database/sql"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

func openSettingsWorld(t *testing.T) (*database.Store, *secrets.Vault) {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	v, err := secrets.Open(context.Background(), s, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	return s, v
}

// EnsureDownloader adds a Downloader once per address, so a restart does not
// add the lab one again.
func TestEnsureDownloaderOncePerURL(t *testing.T) {
	ctx := context.Background()
	s, _ := openSettingsWorld(t)
	d := domain.Downloader{Name: "Lab qBittorrent", Adapter: "qbittorrent", Protocol: "torrent", URL: "http://lab:8080", PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok"}
	count := func() int {
		var n int
		if err := s.Read().QueryRow("SELECT count(*) FROM downloaders").Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	for i := 0; i < 2; i++ {
		if err := s.Write(ctx, func(tx *sql.Tx) error { return EnsureDownloader(tx, d) }); err != nil {
			t.Fatal(err)
		}
	}
	if n := count(); n != 1 {
		t.Fatalf("rows %d", n)
	}
	d.URL, d.Name = "http://lab2:8080", "Lab 2"
	if err := s.Write(ctx, func(tx *sql.Tx) error { return EnsureDownloader(tx, d) }); err != nil {
		t.Fatal(err)
	}
	if n := count(); n != 2 {
		t.Fatalf("rows %d", n)
	}
}
