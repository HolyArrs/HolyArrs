package health

import (
	"strings"
	"testing"
)

// Two Downloaders on one SABnzbd (one with the /sabnzbd path) make each job
// show twice: the newer one gets a warning; removing it clears the line.
func TestSameDownloaderCheck(t *testing.T) {
	w := newWorld(t)
	add := func(name, url string) {
		w.exec(t, `INSERT INTO downloaders (created_at, created_by, updated_at, updated_by, name, adapter, protocol, url, category, priority, path_map, after_import, state, last_error)
			VALUES ('2026-10-01T00:00:00Z', '{}', '2026-10-01T00:00:00Z', '{}', ?, 'sabnzbd', 'usenet', ?, 'holyarrs', 2, '[]', 'leave', 'ok', '')`, name, url)
	}
	add("SABnzbd", "http://192.168.1.124:30055")
	add("SABnzbd (moved)", "http://192.168.1.124:30055/sabnzbd")
	add("Other SABnzbd", "http://192.168.1.124:30056")
	cs := w.run(t)
	if _, ok := cs["same_downloader:1"]; ok {
		t.Fatal("the older Downloader gets no line")
	}
	if _, ok := cs["same_downloader:3"]; ok {
		t.Fatal("another port is another app")
	}
	c := cs["same_downloader:2"]
	if c.State != Warning || c.Kind != "same_downloader" || c.Subject != "SABnzbd (moved)" || !strings.Contains(c.Message, "same app as SABnzbd") {
		t.Fatalf("twin: %+v", c)
	}
	w.exec(t, `UPDATE downloaders SET removed = 1 WHERE id = 2`)
	if _, ok := w.run(t)["same_downloader:2"]; ok {
		t.Fatal("a removed twin still warns")
	}
}

// Two Downloaders on one qBittorrent that each show only their own
// categories, with none in common, are kept apart and get no line. Overlap
// in any category (after-import included) or one without the fence warns.
func TestSameDownloaderFencedCategories(t *testing.T) {
	w := newWorld(t)
	add := func(name, cat, after string, own bool) {
		w.exec(t, `INSERT INTO downloaders (created_at, created_by, updated_at, updated_by, name, adapter, protocol, url, category, priority, path_map, after_import, state, last_error, category_after_import, own_only)
			VALUES ('2026-10-01T00:00:00Z', '{}', '2026-10-01T00:00:00Z', '{}', ?, 'qbittorrent', 'torrent', 'http://qbit:8080', ?, 2, '[]', 'leave', 'ok', '', ?, ?)`, name, cat, after, own)
	}
	add("HDD", "holyarrs", "holyarrs-done", true)
	add("USB", "holyarrs_usb", "holyarrs_usb-done", true)
	if _, ok := w.run(t)["same_downloader:2"]; ok {
		t.Fatal("fenced, disjoint categories still warn")
	}
	w.exec(t, `UPDATE downloaders SET category_after_import = 'HolyArrs' WHERE id = 2`)
	if _, ok := w.run(t)["same_downloader:2"]; !ok {
		t.Fatal("an after-import category shared with the other must warn")
	}
	w.exec(t, `UPDATE downloaders SET category_after_import = '', own_only = 0 WHERE id = 2`)
	if _, ok := w.run(t)["same_downloader:2"]; !ok {
		t.Fatal("a Downloader that sees every category must warn")
	}
}

// two fenced Downloaders on one qBittorrent that still share one
// category (here the music one, which neither page row shows by default)
// get a line that names that category and says to change it, not to remove
// a Downloader: removing one would break tag routing.
func TestSameDownloaderNamesSharedCategory(t *testing.T) {
	w := newWorld(t)
	add := func(name, cat, shows, movies string) {
		w.exec(t, `INSERT INTO downloaders (created_at, created_by, updated_at, updated_by, name, adapter, protocol, url, category, priority, path_map, after_import, state, last_error, category_shows, category_movies, category_music, own_only)
			VALUES ('2026-10-01T00:00:00Z', '{}', '2026-10-01T00:00:00Z', '{}', ?, 'qbittorrent', 'torrent', 'http://qbit:8080', ?, 2, '[]', 'leave', 'ok', '', ?, ?, 'music', 1)`, name, cat, shows, movies)
	}
	add("qBittorrent", "holyarrs", "TV Shows", "Movies")
	add("qBittorrent - Anime", "holyarrs-anime", "Anime", "Anime Movies")
	c, ok := w.run(t)["same_downloader:2"]
	if !ok {
		t.Fatal("a shared category is a real overlap and must still warn")
	}
	if !strings.Contains(c.Message, `category "music"`) || !strings.Contains(c.Message, "qBittorrent") || strings.Contains(c.Message, "Remove one") {
		t.Fatalf("the line must name the shared category: %q", c.Message)
	}
	w.exec(t, `UPDATE downloaders SET category_music = 'anime-music' WHERE id = 2`)
	if _, ok := w.run(t)["same_downloader:2"]; ok {
		t.Fatal("giving the twin its own music category must clear the line")
	}
}
