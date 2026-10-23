package downloader

import (
	"testing"
	"time"
)

// Cache audit 2026-10-23 (other defaults): a busy download client (a NAS
// during a recheck) gets 30 s, not 15.
func TestDownloaderTimeout30s(t *testing.T) {
	for name, got := range map[string]time.Duration{
		"client":       Client("http://qbittorrent:8090", true).Timeout,
		"qbittorrent":  NewQbittorrent("http://x", nil).http.Timeout,
		"sabnzbd":      NewSabnzbd("http://x", "k", nil).http.Timeout,
		"nzbget":       NewNZBGet("http://x", "", "", nil).http.Timeout,
		"deluge":       NewDeluge("http://x", "", nil).http.Timeout,
		"transmission": NewTransmission("http://x", "", "", nil).http.Timeout,
	} {
		if got != 30*time.Second {
			t.Fatalf("%s timeout %v", name, got)
		}
	}
}
