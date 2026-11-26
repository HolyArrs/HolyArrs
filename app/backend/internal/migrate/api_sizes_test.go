package migrate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A Radarr read over its address and key brings its size table too, not
// only a backup.
func TestAPIReadBringsSizeTable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v3/system/status":
			_, _ = w.Write([]byte(`{"appName":"Radarr","version":"6.4.4"}`))
		case "/api/v3/movie":
			_, _ = w.Write([]byte(`[]`))
		case "/api/v3/qualitydefinition":
			_, _ = w.Write([]byte(`[{"quality":{"id":7,"name":"Bluray-1080p"},"title":"Bluray-1080p","minSize":21,"maxSize":99,"id":22},
				{"quality":{"id":18,"name":"WEBDL-2160p"},"title":"WEBDL-2160p","minSize":35,"id":23}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	s, err := (arrReader{kind: Radarr}).Read(context.Background(), Source{Kind: Radarr, URL: srv.URL, Key: "k", AllowLocal: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.QualitySizes) != 2 {
		t.Fatalf("want 2 size rows, got %+v", s.QualitySizes)
	}
	if q := s.QualitySizes[0]; q.Quality != "Bluray-1080p" || q.Min != 21 || q.Max != 99 {
		t.Fatalf("first row %+v", q)
	}
	if q := s.QualitySizes[1]; q.Quality != "WEBDL-2160p" || q.Min != 35 || q.Max != 0 {
		t.Fatalf("open max row %+v", q)
	}
}
