package downloads

import (
	"strings"
	"testing"
)

func TestGrabLinkAllowedSourcePolicy(t *testing.T) {
	for _, l := range []string{"http://192.168.1.1/cgi-bin/x", "http://169.254.169.254/latest/meta-data/", "http://localhost:8080/a"} {
		if _, err := grabLinkAllowed("qbittorrent", l, false); err == nil {
			t.Errorf("qbittorrent got %s", l)
		}
		if _, err := grabLinkAllowed("sabnzbd", l, false); err == nil {
			t.Errorf("sabnzbd got %s", l)
		}
	}
	if _, err := grabLinkAllowed("qbittorrent", "http://192.168.1.1/x.torrent", true); err != nil {
		t.Errorf("local Source refused: %v", err)
	}
	if got, err := grabLinkAllowed("sabnzbd", "https://indexer.example/get?id=1", false); err != nil || got != "https://indexer.example/get?id=1" {
		t.Errorf("public link changed: %q %v", got, err)
	}
	m := "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&tr=http://10.0.0.5/announce&tr=https://tracker.example/announce"
	got, err := grabLinkAllowed("qbittorrent", m, false)
	if err != nil || strings.Contains(got, "10.0.0.5") || !strings.Contains(got, "tracker.example") {
		t.Errorf("magnet not cleaned: %q %v", got, err)
	}
}
