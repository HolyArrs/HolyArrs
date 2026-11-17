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
