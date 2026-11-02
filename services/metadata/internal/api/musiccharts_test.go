package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMusicChartsQueries(t *testing.T) {
	s := &Server{}
	for _, c := range []struct {
		url  string
		code int
		fn   func(*Server, http.ResponseWriter, *http.Request)
	}{
		{"/v1/music/charts?kind=videos", 400, (*Server).serveMusicCharts},
		{"/v1/music/charts?region=usa", 400, (*Server).serveMusicCharts},
		{"/v1/music/charts?genre=Pop!", 400, (*Server).serveMusicCharts},
		{"/v1/music/charts?x=1", 400, (*Server).serveMusicCharts},
		{"/v1/music/charts?kind=songs", 404, (*Server).serveMusicCharts},
		{"/v1/music/genres?region=US", 404, (*Server).serveMusicGenres},
		{"/v1/music/suggest?q=radi", 404, (*Server).serveMusicSuggest},
		{"/v1/music/releases?from=2026-10-30&to=2026-10-01", 400, (*Server).serveMusicReleases},
		{"/v1/music/releases?artists=nope", 400, (*Server).serveMusicReleases},
		{"/v1/music/releases", 404, (*Server).serveMusicReleases},
	} {
		w := httptest.NewRecorder()
		c.fn(s, w, httptest.NewRequest("GET", c.url, nil))
		if w.Code != c.code {
			t.Errorf("%s: %d, want %d", c.url, w.Code, c.code)
		}
	}
}
