package downloads

import (
	"context"
	"database/sql"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// NZBGet answers with its own spelling of a category ("Movies" for movies),
// so the reader's filters must still keep the job, or its row waits forever.
func TestCategoryFiltersIgnoreCase(t *testing.T) {
	jobs := []Job{{ID: "a", Category: "Movies"}, {ID: "b", Category: "other"}}
	if got := inCategory(jobs, []string{"movies"}); len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("inCategory kept %v", got)
	}
	v := SyncView{Items: map[string]Job{"a": jobs[0], "b": jobs[1]}}
	if got := snapshotJobs(v, []string{"movies"}); len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("snapshotJobs kept %v", got)
	}
	s := &MainState{jobs: map[string]Job{"a": jobs[0], "b": jobs[1]}}
	if got := s.Jobs("movies"); len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("Jobs kept %v", got)
	}
}

// A category spelled twice in different case is one category.
func TestCategoriesListACaseOnlyRepeatOnce(t *testing.T) {
	d := domain.Downloader{Category: "movies", CategoryMovies: "Movies", CategoryShows: "tv"}
	if got := d.Categories(); len(got) != 2 || got[0] != "movies" || got[1] != "tv" {
		t.Fatalf("categories %v", got)
	}
	if got := otherCategories(d); len(got) != 1 || got[0] != "tv" {
		t.Fatalf("other categories %v", got)
	}
}

// SABnzbd keeps category names in lower case, so "Movies" typed in HolyArrs
// is its "movies".
func TestSabnzbdHasCategoryIgnoresCase(t *testing.T) {
	u := startSab(t, &fakeSab{})
	ok, err := NewSabnzbd(u, sabKey, nil).HasCategory(context.Background(), "Movies")
	if err != nil || !ok {
		t.Fatalf("has Movies: %v %v", ok, err)
	}
}

// Fixing only the case of the category is not a move to another category,
// so it is allowed while downloads run.
func TestChangeAllowsACaseOnlyCategoryFixWhileActive(t *testing.T) {
	ctx := context.Background()
	s, v := openSettingsWorld(t)
	cat := "movies"
	d, err := Add(ctx, s, v, Input{Name: "n", Adapter: "nzbget", URL: "http://10.0.0.5:6789", Category: &cat}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		dl := &domain.Download{DownloaderID: d.ID, SlotIDs: []int64{}, ExternalID: "abc", State: "sent", FilesSeen: []domain.SeenFile{}, StartedBy: domain.System}
		_, err := database.Insert(tx, "downloads", dl, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	upper := "Movies"
	if _, err := Change(ctx, s, v, d.ID, Input{Name: "n", Adapter: "nzbget", URL: d.URL, Category: &upper}, domain.System); err != nil {
		t.Fatalf("case-only change: %v", err)
	}
}

// Deluge keeps labels in lower case, so a Downloader set to "Movies" still
// owns a torrent labelled "movies" and may remove it.
func TestDelugeRemoveIgnoresLabelCase(t *testing.T) {
	torrent := fakeDelugeTorrent("Downloading", 25)
	torrent["label"] = "movies"
	f := &fakeDeluge{connected: true, labels: map[string]bool{"movies": true}, torrents: map[string]map[string]any{delugeHash: torrent}}
	d := NewDeluge(startDeluge(t, f), f.password, nil)
	d.Category = "Movies"
	if err := d.Remove(context.Background(), delugeHash, true); err != nil {
		t.Fatalf("remove own torrent: %v", err)
	}
}
