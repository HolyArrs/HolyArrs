package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// While the chart gate is closed, charts, genres, new releases and the
// trending lists answer "building" (kept a minute at the edge); once it
// opens they answer as before.
func TestChartsHiddenWhileGateClosed(t *testing.T) {
	ready := false
	s := &Server{opts: Options{ChartsReady: func() bool { return ready }}}
	routes := []struct {
		url string
		fn  func(*Server, http.ResponseWriter, *http.Request)
	}{
		{"/v1/music/charts?kind=albums", (*Server).serveMusicCharts},
		{"/v1/music/genres", (*Server).serveMusicGenres},
		{"/v1/music/new?window=week", (*Server).serveMusicNew},
		{"/v1/lists/music-trending?kind=album", (*Server).serveMusicTrending},
		{"/v1/music/lists/shelves", (*Server).serveMusicShelves},
	}
	for _, c := range routes {
		w := httptest.NewRecorder()
		c.fn(s, w, httptest.NewRequest("GET", c.url, nil))
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"building"`) || w.Header().Get(partialHeader) == "" {
			t.Errorf("%s closed: %d %s", c.url, w.Code, w.Body.String())
		}
		ew := &edgeWriter{ResponseWriter: httptest.NewRecorder(), policy: edgePolicy(c.url)}
		c.fn(s, ew, httptest.NewRequest("GET", c.url, nil))
		if got := ew.Header().Get("Cache-Control"); got != partialPolicy {
			t.Errorf("%s building kept at the edge as %q", c.url, got)
		}
	}
	ready = true
	for _, c := range routes {
		w := httptest.NewRecorder()
		c.fn(s, w, httptest.NewRequest("GET", c.url, nil))
		if strings.Contains(w.Body.String(), "building") {
			t.Errorf("%s still building with the gate open", c.url)
		}
	}
}
