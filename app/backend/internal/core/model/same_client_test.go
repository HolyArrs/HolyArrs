package model

import "testing"

func TestSameClient(t *testing.T) {
	sab := func(u string) Downloader { return Downloader{Adapter: "sabnzbd", URL: u} }
	for _, c := range []struct {
		a, b Downloader
		want bool
	}{
		{sab("http://192.168.1.124:30055"), sab("http://192.168.1.124:30055/sabnzbd"), true},
		{sab("http://SAB:8080/"), sab("https://sab:8080/sabnzbd/"), true},
		{sab("sab:8080"), sab("http://sab:8080"), true},
		{sab("http://sab"), sab("http://sab:80/sabnzbd"), true},
		{sab("http://sab:8080"), sab("http://sab:8081"), false},
		{sab("http://sab:8080"), Downloader{Adapter: "qbittorrent", URL: "http://sab:8080"}, false},
		{sab(""), sab(""), false},
		{Downloader{Adapter: "qbittorrent", URL: "https://proxy/qbit-movies"}, Downloader{Adapter: "qbittorrent", URL: "https://proxy:443/qbit-tv/"}, false},
		{Downloader{Adapter: "qbittorrent", URL: "https://proxy/qbit-tv"}, Downloader{Adapter: "qbittorrent", URL: "https://PROXY:443/qbit-tv/"}, true},
	} {
		if got := SameClient(c.a, c.b); got != c.want {
			t.Errorf("SameClient(%q, %q) = %v, want %v", c.a.URL, c.b.URL, got, c.want)
		}
	}
}
