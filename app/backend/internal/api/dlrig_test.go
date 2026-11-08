package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

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
	srv := httptest.NewServer(New(Server{Store: s, Metadata: metadata.Lab(), Version: "test", Secrets: v}).Handler())
	t.Cleanup(srv.Close)
	return srv, s
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
