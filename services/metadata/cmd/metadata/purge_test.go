package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CoderLuii/holyarrs-metadata/internal/api"
)

// One command purges every shared text prefix; pictures only when asked.
func TestPurgeEdgeClearsEveryPrefix(t *testing.T) {
	var got []string
	cf := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer synthetic-cf-token" {
			w.WriteHeader(403)
			return
		}
		switch {
		case r.Method == "GET" && r.URL.Path == "/zones" && r.URL.Query().Get("name") == "example.dev":
			_, _ = w.Write([]byte(`{"success":true,"result":[{"id":"zone1"}]}`))
		case r.Method == "POST" && r.URL.Path == "/zones/zone1/purge_cache":
			var b struct{ Prefixes []string }
			_ = json.NewDecoder(r.Body).Decode(&b)
			got = append(got, b.Prefixes...)
			_, _ = w.Write([]byte(`{"success":true}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer cf.Close()
	t.Setenv("CLOUDFLARE_API_TOKEN", "synthetic-cf-token")
	if err := run([]string{"purge-edge", "-api", cf.URL, "-zone", "example.dev", "-host", "api.example.dev"}); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(api.EdgePrefixes())-1 || strings.Contains(strings.Join(got, " "), "/v1/img/") {
		t.Fatalf("purged %v (never the versioned pictures)", got)
	}
	got = nil
	if err := run([]string{"purge-edge", "-api", cf.URL, "-zone", "example.dev", "-host", "api.example.dev", "-pictures"}); err != nil || len(got) != len(api.EdgePrefixes()) {
		t.Fatalf("with -pictures: %v %v", err, got)
	}
	for _, p := range got {
		if !strings.HasPrefix(p, "api.example.dev/v1/") {
			t.Fatalf("prefix %q", p)
		}
	}
	got = nil
	if err := run([]string{"purge-edge", "-api", cf.URL, "-zone", "example.dev", "-host", "api.example.dev", "-only", "/v1/music/,/v1/img/album/"}); err != nil ||
		strings.Join(got, " ") != "api.example.dev/v1/music/ api.example.dev/v1/img/album/" {
		t.Fatalf("with -only: %v %v", err, got)
	}
	if err := run([]string{"purge-edge", "-api", cf.URL, "-zone", "example.dev", "-only", "music"}); err == nil {
		t.Fatal("-only without /v1/ accepted")
	}
	t.Setenv("CLOUDFLARE_API_TOKEN", "")
	if err := run([]string{"purge-edge", "-api", cf.URL}); err == nil {
		t.Fatal("no token accepted")
	}
}
