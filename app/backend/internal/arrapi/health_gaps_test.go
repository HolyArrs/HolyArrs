package arrapi

import (
	"encoding/json"
	"strings"
	"testing"
)

func readHealth(t *testing.T, r *pinRig) map[string][]healthItem {
	t.Helper()
	out := map[string][]healthItem{}
	for side, b := range r.both("health") {
		var got []healthItem
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("%s: %v %s", side, err, b)
		}
		out[side] = got
	}
	return out
}

// Once setup is over (something is in the Library), a missing or all-off
// Source or Downloader is a problem on /health, as Home counts it.
func TestHealthSetupGaps(t *testing.T) {
	r := newPinRig(t)
	// Before setup is over the welcome card covers it: nothing listed.
	for side, got := range readHealth(t, r) {
		if len(got) != 0 {
			t.Fatalf("%s before setup: %+v", side, got)
		}
	}
	r.movie("Alpha", "500", 1)
	for side, got := range readHealth(t, r) {
		if len(got) != 2 || got[0].Source != "IndexerCheck" || got[1].Source != "DownloadClientCheck" || got[0].Type != "warning" {
			t.Fatalf("%s none added: %+v", side, got)
		}
	}
	r.exec(`INSERT INTO sources (created_at, created_by, updated_at, updated_by, name, protocol, format, url, state) VALUES (?, '{}', ?, '{}', 'Fake', 'torrent', 'torznab', 'http://x', 'disabled')`, pinStamp, pinStamp)
	r.exec(`INSERT INTO downloaders (created_at, created_by, updated_at, updated_by, name, adapter, protocol, url) VALUES (?, '{}', ?, '{}', 'qb', 'qbittorrent', 'torrent', 'http://x')`, pinStamp, pinStamp)
	for side, got := range readHealth(t, r) {
		if len(got) != 1 || got[0].Source != "IndexerCheck" || !strings.Contains(strings.ToLower(got[0].Message), "off") {
			t.Fatalf("%s source off: %+v", side, got)
		}
	}
	// Sources turned off as a part: Home asks for none.
	r.exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('this_app', 'parts_off', '["sources"]', ?, '{}', ?, '{}')`, pinStamp, pinStamp)
	for side, got := range readHealth(t, r) {
		if len(got) != 0 {
			t.Fatalf("%s sources part off: %+v", side, got)
		}
	}
}

// A Source or Downloader already in a failing state before the next check run
// is one problem on /health, as Home counts it; once a check of that kind
// fails, the check is the row instead.
func TestHealthPartTrouble(t *testing.T) {
	r := newPinRig(t)
	r.movie("Alpha", "500", 1)
	r.exec(`INSERT INTO sources (created_at, created_by, updated_at, updated_by, name, protocol, format, url, state) VALUES (?, '{}', ?, '{}', 'Fake', 'torrent', 'torznab', 'http://x', 'paused')`, pinStamp, pinStamp)
	r.exec(`INSERT INTO downloaders (created_at, created_by, updated_at, updated_by, name, adapter, protocol, url, state) VALUES (?, '{}', ?, '{}', 'qb', 'qbittorrent', 'torrent', 'http://x', 'unreachable')`, pinStamp, pinStamp)
	for side, got := range readHealth(t, r) {
		if len(got) != 2 || got[0].Source != "IndexerCheck" || got[1].Source != "DownloadClientCheck" {
			t.Fatalf("%s part trouble: %+v", side, got)
		}
	}
	r.exec(`INSERT INTO health_checks (check_key, kind, subject, state, message, since, checked_at, streak, confirmed) VALUES ('downloader:1', 'downloader', 'qb', 'warning', 'Downloader qb does not answer', ?, ?, 1, 'warning')`, pinStamp, pinStamp)
	for side, got := range readHealth(t, r) {
		if len(got) != 2 || got[0].Source != "DownloaderCheck" || got[1].Source != "IndexerCheck" {
			t.Fatalf("%s check is the row: %+v", side, got)
		}
	}
}
