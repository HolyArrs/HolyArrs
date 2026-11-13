package health

import (
	"context"
	"strings"
	"testing"
)

// Usenet Sources with only qBittorrent: Health and doctor warn that no Usenet
// Downloader is set up; adding one clears it. A protocol no Source gives is
// not checked.
func TestNoDownloaderForTheSourcesProtocolWarns(t *testing.T) {
	w := newWorld(t)
	w.addDownloader(t, "qBittorrent", "ok")
	w.exec(t, `INSERT INTO sources (created_at, created_by, updated_at, updated_by, name, protocol, format, url, use_for, kinds, categories, limits, priority, state, pause_reason, extra)
		VALUES ('2026-10-01T00:00:00Z', '{}', '2026-10-01T00:00:00Z', '{}', 'NZB', 'usenet', 'newznab', 'http://x/api', '{}', '[]', '{}', '{}', 25, 'enabled', NULL, '{}')`)
	cs := w.run(t)
	c := cs["no_downloader:usenet"]
	if c.State != Warning || c.Kind != "no_downloader" || c.Subject != "usenet" || !strings.Contains(c.Message, "no Usenet Downloader is set up") {
		t.Fatalf("usenet check %+v", c)
	}
	if _, ok := cs["no_downloader:torrent"]; ok {
		t.Fatal("checked torrents with no torrent Source")
	}
	var r Report
	downloaderKindChecks(context.Background(), &r, w.s.Read())
	if len(r.Checks) != 1 || r.Checks[0].Status != StatusWarn || !strings.Contains(r.Checks[0].Text, "no Usenet Downloader") || !strings.Contains(r.Checks[0].Fix, "SABnzbd") {
		t.Fatalf("doctor %+v", r.Checks)
	}
	w.exec(t, `INSERT INTO downloaders (created_at, created_by, updated_at, updated_by, name, adapter, protocol, url, category, priority, path_map, after_import, state, last_error)
		VALUES ('2026-10-01T00:00:00Z', '{}', '2026-10-01T00:00:00Z', '{}', 'SAB', 'sabnzbd', 'usenet', 'http://sab', 'holyarrs', 1, '[]', 'leave', 'ok', '')`)
	if c := w.run(t)["no_downloader:usenet"]; c.State != OK {
		t.Fatalf("still warns with SABnzbd added: %+v", c)
	}
	r = Report{}
	downloaderKindChecks(context.Background(), &r, w.s.Read())
	if len(r.Checks) != 0 {
		t.Fatalf("doctor still warns %+v", r.Checks)
	}
}
