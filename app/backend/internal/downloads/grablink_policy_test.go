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
	t.Setenv(TrackerURLPrefixesEnv, "iptv://")
	iptv := "magnet:?xt=urn:btih:b791a0df00000000000000000000000000000000&dn=Landman.S02E09&tr=iptv://b791a0df00000000000000000000000000000000/streamq/landman"
	got, err = grabLinkAllowed("qbittorrent", iptv, false)
	if err != nil || !strings.Contains(got, "tr=iptv%3A%2F%2F") {
		t.Errorf("iptv tracker stripped: %q %v", got, err)
	}
}

func TestTrackerURLPrefixesEnv(t *testing.T) {
	iptv := "iptv://b791a0df00000000000000000000000000000000/streamq/landman"
	custom := "facade://guid/path"

	t.Setenv(TrackerURLPrefixesEnv, "")
	if trackerAllowed(iptv, false) {
		t.Errorf("empty %s still allowed iptv://", TrackerURLPrefixesEnv)
	}

	t.Setenv(TrackerURLPrefixesEnv, "facade://, other://")
	if trackerAllowed(iptv, false) {
		t.Errorf("iptv:// allowed when env lists other prefixes")
	}
	if !trackerAllowed(custom, false) {
		t.Errorf("facade:// refused when listed in %s", TrackerURLPrefixesEnv)
	}

	t.Setenv(TrackerURLPrefixesEnv, "IPTV://")
	if !trackerAllowed(iptv, false) {
		t.Errorf("case-insensitive prefix match failed")
	}
}
