package downloads

import (
	"encoding/json"
	"net/http/httptest"
	"slices"
	"testing"
)

// catsOf lists the categories a saved Downloader uses, main first.
func catsOf(d DownloaderView) []string {
	out := []string{d.Category}
	for _, c := range []string{d.CategoryShows, d.CategoryMovies, d.CategoryMusic} {
		if c != "" {
			out = append(out, c)
		}
	}
	return out
}

// One qBittorrent, two Downloaders split by tag, each showing only
// its own categories. The auto-pick must not hand the newer Downloader a
// category the older one already lists: that shared category makes the same
// jobs show on both and keeps the same_downloader health line up, and the
// person never chose the pick. The kind falls back to the Downloader's own
// main category instead, which no twin uses.
func TestAutoPickSkipsTwinCategories(t *testing.T) {
	t.Parallel()
	srv, _ := dlServer(t)
	key := "0123456789abcdef0123456789abcdef"
	f := newFake()
	f.listCats = true
	f.key = key
	f.cats = []string{"tv", "movies", "music"}
	q := httptest.NewServer(f)
	t.Cleanup(q.Close)
	add := func(name, main string, tags []string, own bool) DownloaderView {
		t.Helper()
		var d DownloaderView
		_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloaders", body(map[string]any{
			"name": name, "kind": "qbittorrent", "url": q.URL, "category": main, "secret": key,
			"tags": tags, "own_only": own, "allow_local_network": true}), 201), &d)
		return d
	}
	main := add("qBittorrent", "holyarrs", []string{}, true)
	if main.CategoryShows != "tv" || main.CategoryMovies != "movies" || main.CategoryMusic != "music" {
		t.Fatalf("the first Downloader picks the client's categories: %+v", main)
	}
	anime := add("qBittorrent - Anime", "holyarrs-anime", []string{"anime", "anime-movies"}, true)
	if anime.CategoryShows != "" || anime.CategoryMovies != "" || anime.CategoryMusic != "" {
		t.Fatalf("the twin was given a category the first one lists: %+v", anime)
	}
	for _, c := range catsOf(main) {
		if slices.ContainsFunc(catsOf(anime), func(x string) bool { return sameCategory(x, c) }) {
			t.Fatalf("the pair share the category %q", c)
		}
	}
}

// A free category that also fits the kind is still picked, so skipping a
// twin's does not throw the pick away.
func TestAutoPickTakesTheNextFreeFit(t *testing.T) {
	t.Parallel()
	srv, _ := dlServer(t)
	key := "0123456789abcdef0123456789abcdef"
	f := newFake()
	f.listCats = true
	f.key = key
	f.cats = []string{"music", "lidarr"}
	q := httptest.NewServer(f)
	t.Cleanup(q.Close)
	add := func(name, main string) DownloaderView {
		t.Helper()
		var d DownloaderView
		_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloaders", body(map[string]any{
			"name": name, "kind": "qbittorrent", "url": q.URL, "category": main, "secret": key,
			"allow_local_network": true}), 201), &d)
		return d
	}
	if first := add("qBittorrent", "holyarrs"); first.CategoryMusic != "music" {
		t.Fatalf("first picks: %+v", first)
	}
	if twin := add("qBittorrent - Anime", "holyarrs-anime"); twin.CategoryMusic != "lidarr" {
		t.Fatalf("the twin takes the other fitting category, not the shared one: %+v", twin)
	}
}
