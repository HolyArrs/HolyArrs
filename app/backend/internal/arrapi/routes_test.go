package arrapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serveGET(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec
}

// serve folds case and a trailing slash and puts the version header on
// every answer. Outside the API it answers a bare 404, because Bazarr
// reads a JSON body there as a version.
func TestServeRewritesPaths(t *testing.T) {
	h := Sonarr(Deps{Store: settingsStore(t)})

	rec := serveGET(h, "/sonarr/api/system/status")
	if rec.Code != 404 || rec.Body.Len() != 0 || rec.Header().Get("X-Application-Version") != SonarrVersion {
		t.Fatalf("outside the API: %d %q %v", rec.Code, rec.Body, rec.Header())
	}
	rec = serveGET(h, "/sonarr/api/v3/System/Status/")
	if rec.Code != 200 || rec.Header().Get("X-Application-Version") != SonarrVersion {
		t.Fatalf("case or slash not folded: %d %s", rec.Code, rec.Body)
	}
	rec = serveGET(h, "/sonarr/api/v3/nope")
	if rec.Code != 404 || !strings.Contains(rec.Body.String(), `"message"`) || rec.Header().Get("X-Application-Version") != SonarrVersion {
		t.Fatalf("unknown route: %d %s", rec.Code, rec.Body)
	}
	// Covers also answer right after the side. With no picture source they
	// give a 404 with a message, not the bare outside-the-API answer.
	rec = serveGET(h, "/sonarr/MediaCover/1/poster.jpg")
	if rec.Code != 404 || !strings.Contains(rec.Body.String(), "message") {
		t.Fatalf("direct mediacover: %d %q", rec.Code, rec.Body)
	}
	r := Radarr(Deps{Store: settingsStore(t)})
	if rec = serveGET(r, "/radarr/api/v3/system/status"); rec.Code != 200 || rec.Header().Get("X-Application-Version") != RadarrVersion {
		t.Fatalf("radarr: %d %v", rec.Code, rec.Header())
	}
}
