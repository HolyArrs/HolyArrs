package downloads

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// webRig serves the Downloads routes alone, without sign-in, the way api
// registers them.
func webRig(t *testing.T, a *Web) *httptest.Server {
	t.Helper()
	if a.Library == nil {
		a.Library = &library.Service{Store: a.Store, Queue: a.Queue, Metadata: a.Metadata, Live: a.Live}
	}
	if a.Search == nil {
		a.Search = &sources.Searcher{Store: a.Store, Secrets: a.Secrets, Queue: a.Queue}
		if a.Live != nil {
			a.Search.Live = a.Live // a nil hub must stay a nil Publisher
		}
	}
	m := http.NewServeMux()
	a.Routes(m)
	srv := httptest.NewServer(m)
	t.Cleanup(srv.Close)
	return srv
}

func do(t *testing.T, method, url, body string, want int) []byte {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != want {
		t.Fatalf("%s %s: %d, want %d: %s", method, url, resp.StatusCode, want, b)
	}
	return b
}

func body(m map[string]any) string {
	b, _ := json.Marshal(m)
	return string(b)
}

func dlServer(t *testing.T) (*httptest.Server, *database.Store) {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	v, err := secrets.Open(context.Background(), s, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	return webRig(t, &Web{Store: s, Metadata: metadata.Lab(), Secrets: v}), s
}

// fakeSAB answers the version, queue and categories calls with one key.
func fakeSAB(t *testing.T, key string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch {
		case q.Get("mode") == "version":
			_, _ = w.Write([]byte(`{"version":"5.1.3"}`))
		case q.Get("apikey") != key:
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("API Key Incorrect"))
		case q.Get("mode") == "get_cats":
			_, _ = w.Write([]byte(`{"categories":["*","movies"]}`))
		default:
			_, _ = w.Write([]byte(`{"queue":{"status":"Idle","slots":[]}}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}
