package migrate

import "testing"

// TestSeerrRequest_UnknownKindSkipped keeps a request with no known media
// kind out instead of bringing it over as a movie.
func TestSeerrRequest_UnknownKindSkipped(t *testing.T) {
	for kind, want := range map[string]string{"movie": "movie", "tv": "show", "": "", "music": ""} {
		rq, ok := seerrRequest(1, 2, kind, map[string]any{"tmdbId": 5}, 1, 0, false, "", "")
		if ok != (want != "") || ok && rq.Kind != want {
			t.Errorf("%q: kind %q ok %v, want %q", kind, rq.Kind, ok, want)
		}
	}
}
