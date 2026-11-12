package library

import "testing"

// TestPictureSourcesTVMaze: TVmaze posters come through on the exact
// https host only.
func TestPictureSourcesTVMaze(t *testing.T) {
	c := &ArtworkCache{Allowed: PictureSources("https://api.example.test")}
	for _, u := range []string{
		"https://static.tvmaze.com/uploads/images/original_untouched/1/4600.jpg",
		"https://image.tmdb.org/t/p/w342/x.jpg",
		"https://api.example.test/v1/img/tmdb/w342/x.jpg",
	} {
		if !c.allowed(u) {
			t.Errorf("refused %s", u)
		}
	}
	for _, u := range []string{
		"http://static.tvmaze.com/uploads/a.jpg",
		"https://static.tvmaze.com.evil.test/a.jpg",
		"https://static.tvmaze.com@evil.test/a.jpg",
		"https://evilstatic.tvmaze.com/a.jpg",
		"https://static.tvmaze.com:8443/a.jpg",
		"https://static.tvmaze.com/../a.jpg",
	} {
		if c.allowed(u) {
			t.Errorf("allowed %s", u)
		}
	}
}
