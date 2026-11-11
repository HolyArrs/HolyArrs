package downloads

import (
	"testing"
	"time"
)

func TestDownloaderTimeout30s(t *testing.T) {
	for name, got := range map[string]time.Duration{
		"client":       Client("http://qbittorrent:8090", true).Timeout,
		"qbittorrent":  NewQbittorrent("http://x", nil).http.Timeout,
		"sabnzbd":      NewSabnzbd("http://x", "k", nil).http.Timeout,
		"nzbget":       NewNZBGet("http://x", "", "", nil).http.Timeout,
		"deluge":       NewDeluge("http://x", "", nil).http.Timeout,
		"rtorrent":     NewRTorrent("http://x", "", "", nil).http.Timeout,
		"transmission": NewTransmission("http://x", "", "", nil).http.Timeout,
	} {
		if got != 30*time.Second {
			t.Fatalf("%s timeout %v", name, got)
		}
	}
}
