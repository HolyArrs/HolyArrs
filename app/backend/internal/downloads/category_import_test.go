package downloads

import (
	"context"
	"database/sql"
	"slices"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// An imported Downloader meets its client on the first check: kinds the old
// apps left empty take the client's fitting categories, the one it brought
// stays, and a missing main category is made.
func TestSettleImportedFillsEmptyKinds(t *testing.T) {
	t.Parallel()
	_, s := dlServer(t)
	key := "0123456789abcdef0123456789abcdef"
	sab, cats := sabCats(t, key, "Shows", "radarr")
	row := domain.Downloader{Name: "Imported", Adapter: "sabnzbd", Protocol: "usenet", URL: sab.URL, Category: "sab-main", State: "ok",
		CategoryShows: "tv-old", CategoriesImported: []string{"tv-old", "sab-main"}, AfterImport: "leave", Writable: true, AllowLocal: true}
	var id int64
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		var err error
		id, err = database.Insert(tx, "downloaders", &row, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	m := &Manager{Store: s, Engine: func(domain.Downloader) (Engine, error) { return NewSabnzbd(sab.URL, key, nil), nil }}
	m.settleImported(context.Background())
	var d domain.Downloader
	if err := database.Get(context.Background(), s.Read(), "downloaders", id, &d); err != nil {
		t.Fatal(err)
	}
	if d.CategoryShows != "tv-old" || d.CategoryMovies != "radarr" || d.CategoryMusic != "" {
		t.Fatalf("kinds %+v", d)
	}
	if got := cats(); !slices.Contains(got, "tv-old") || !slices.Contains(got, "sab-main") || !slices.Contains(got, "Shows") {
		t.Fatalf("cats %v", got)
	}
	// Once per run: a later change by hand is not undone.
	_ = s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE downloaders SET category_movies = '' WHERE id = ?", id)
		return err
	})
	m.settleImported(context.Background())
	_ = database.Get(context.Background(), s.Read(), "downloaders", id, &d)
	if d.CategoryMovies != "" {
		t.Fatalf("second pass changed %+v", d)
	}
}
