package migrate

import (
	"database/sql"
	"slices"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A client Sonarr and Radarr share keeps each app's category, per kind, the
// shows one as the main one; HolyArrs may change it and remembers which
// categories the import set, so a check can make a missing one.
func TestImportKeepsEachAppCategory(t *testing.T) {
	qbit := func(k, v string) Provider {
		return Provider{Name: "qBit", Kind: "QBittorrent", Enabled: true, Priority: 1,
			Settings: map[string]any{"host": "qbit", "port": int64(8080), "username": "admin", k: v}, NeedsReentry: []string{"password"}}
	}
	l := newMoveLabWith(t, func(radarr, bazarr, _ *Snapshot) {
		radarr.DownloadClients = []Provider{qbit("movieCategory", "radarr")}
		*bazarr = Snapshot{Kind: Sonarr, Version: "4.0.16", DownloadClients: []Provider{qbit("tvCategory", "tv-sonarr")}}
	})
	if mv := l.move(t, Choices{}); mv.State != "done" {
		t.Fatalf("move %+v", mv)
	}
	rows, err := database.Select[domain.Downloader](t.Context(), l.st.Read(), "downloaders", "1 = 1")
	if err != nil || len(rows) != 1 {
		t.Fatalf("%d Downloaders, %v", len(rows), err)
	}
	d := rows[0]
	if d.Category != "tv-sonarr" || d.CategoryShows != "tv-sonarr" || d.CategoryMovies != "radarr" || d.CategoryMusic != "" || !d.Writable {
		t.Fatalf("categories %+v", d)
	}
	if !slices.Equal(d.CategoriesImported, []string{"tv-sonarr", "radarr"}) {
		t.Fatalf("imported %v", d.CategoriesImported)
	}
}

func TestImportedCategoriesMain(t *testing.T) {
	for _, c := range []struct {
		d    DownloaderPlan
		main string
		all  []string
	}{
		{DownloaderPlan{}, "holyarrs", []string{"holyarrs"}},
		{DownloaderPlan{CategoryMovies: "radarr", CategoryMusic: "lidarr"}, "radarr", []string{"radarr", "lidarr"}},
		{DownloaderPlan{CategoryShows: "tv", CategoryMovies: "TV"}, "tv", []string{"tv"}},
	} {
		main, all := importedCategories(&c.d)
		if main != c.main || !slices.Equal(all, c.all) {
			t.Fatalf("%+v: %s %v", c.d, main, all)
		}
	}
}

// A Downloader a 1.2.0 import made downloads into "holyarrs", which the old
// apps never had. Importing again moves it to the old app's own category.
func TestReimportReplacesHolyarrsCategory(t *testing.T) {
	qbit := Provider{Name: "qBit", Kind: "QBittorrent", Enabled: true, Priority: 1,
		Settings: map[string]any{"host": "qbit", "port": int64(8080), "username": "admin", "movieCategory": "radarr"}, NeedsReentry: []string{"password"}}
	l := newMoveLabWith(t, func(radarr, _, _ *Snapshot) { radarr.DownloadClients = []Provider{qbit} })
	if mv := l.move(t, Choices{}); mv.State != "done" {
		t.Fatalf("move %+v", mv)
	}
	if err := l.st.Write(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE downloaders SET category = 'holyarrs', category_movies = '', categories_imported = '[]', writable = 0")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if mv := l.move(t, Choices{}); mv.State != "done" {
		t.Fatalf("move again %+v", mv)
	}
	rows, err := database.Select[domain.Downloader](t.Context(), l.st.Read(), "downloaders", "1 = 1")
	if err != nil || len(rows) != 1 {
		t.Fatalf("%d Downloaders, %v", len(rows), err)
	}
	if d := rows[0]; d.Category != "radarr" || d.CategoryMovies != "radarr" {
		t.Fatalf("categories %+v", d)
	}
}
