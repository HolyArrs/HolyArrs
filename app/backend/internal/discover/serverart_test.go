package discover

import "testing"

// ServerArtLink turns only the server's own stored addresses into links.
func TestServerArtLink(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"https://api.example.test/v1/img/title-logo/movie-603/1200.png":      "/api/v1/artwork/img/title-logo/movie-603/1200.png",
		"https://api.example.test/v1/img/title-wide/tv-1399/1600.webp?v=a-1": "/api/v1/artwork/img/title-wide/tv-1399/1600.webp?v=a-1",
		"https://api.example.test/v1/img/person/287/500.webp":                "/api/v1/artwork/img/person/287/500.webp",
		"https://api.example.test/v1/img/album/x/500.webp":                   "",
		"https://api.example.test/v1/img/title-logo/movie-603/1200.png?x=1":  "",
		"https://assets.fanart.tv/fanart/movies/603/hdmovielogo/a.png":       "",
		"https://api.example.test/v1/img/title-logo/movie-603/../../a.png":   "",
		"": "",
	} {
		if got := ServerArtLink("https://api.example.test/v1/img", in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}
