package health

import (
	"strings"
	"testing"
)

// A Usenet Downloader gives no file list, so with no save folder
// (HOLYARRS_SAVE_PATH) and no path pair HolyArrs never imports its finished
// jobs. Health names that Downloader and how to fix it; a path pair or
// the save folder makes the warning go.
func TestSaveFolderCheck(t *testing.T) {
	w := newWorld(t)
	w.addDownloader(t, "qBittorrent", "ok")
	w.exec(t, `INSERT INTO downloaders (created_at, created_by, updated_at, updated_by, name, adapter, protocol, url, category, priority, path_map, after_import, state, last_error)
		VALUES ('2026-10-01T00:00:00Z', '{}', '2026-10-01T00:00:00Z', '{}', 'SABnzbd', 'sabnzbd', 'usenet', 'http://sab:8080', 'holyarrs', 2, '[]', 'leave', 'ok', '')`)
	cs := w.run(t)
	if _, ok := cs["save_folder:1"]; ok {
		t.Fatal("a torrent Downloader gives its file list; no save folder check")
	}
	c := cs["save_folder:2"]
	if c.State != Warning || c.Kind != "save_folder" || c.Subject != "SABnzbd" {
		t.Fatalf("Usenet Downloader with no save folder: %+v", c)
	}
	if !strings.Contains(c.Message, "SABnzbd has no save folder set") || !strings.Contains(c.Message, "Settings > Downloaders") {
		t.Fatalf("message: %q", c.Message)
	}
	w.exec(t, `UPDATE downloaders SET path_map = '[{"theirs":"/downloads","ours":"/data/downloads"}]' WHERE id = 2`)
	if c := w.run(t)["save_folder:2"]; c.State != OK || c.Message != "" {
		t.Fatalf("with a path pair: %+v", c)
	}
	w.exec(t, `UPDATE downloaders SET path_map = '[]' WHERE id = 2`)
	w.e.SavePath = "/data/downloads"
	if c := w.run(t)["save_folder:2"]; c.State != OK {
		t.Fatalf("with HOLYARRS_SAVE_PATH set: %+v", c)
	}
}
