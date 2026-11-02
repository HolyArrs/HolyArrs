package api

import (
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeServiceTop map[string]string

func (f fakeServiceTop) List(region string) ([]byte, string, bool) {
	b, ok := f[region]
	return []byte(b), `"etag-` + region + `"`, ok
}

func (f fakeServiceTop) Stats() any { return []string{"ok"} }

func TestServiceTopRoute(t *testing.T) {
	s, _, calls := setup(Options{Burst: 100, ServiceTop: fakeServiceTop{"US": `{"region":"US"}`}})
	const base = "/v1/lists/service-top"
	if w := req(s, base, ""); w.Code == 401 || w.Code == 403 {
		t.Fatalf("shared route refused without a credential %d", w.Code)
	}
	for _, q := range []string{"", "?region=US", "?region=us"} {
		w := req(s, base+q, "synthetic-test-token")
		if w.Code != 200 || w.Body.String() != `{"region":"US"}` || w.Header().Get("ETag") != `"etag-US"` || !strings.HasPrefix(w.Header().Get("Cache-Control"), "public, ") || w.Header().Get("Vary") != "" {
			t.Fatalf("%s: %d %v", q, w.Code, w.Header())
		}
	}
	r := httptest.NewRequest("GET", base, nil)
	r.Header.Set("Authorization", "Bearer synthetic-test-token")
	r.Header.Set("If-None-Match", `"etag-US"`)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, r)
	if rec.Code != 304 {
		t.Fatalf("conditional %d", rec.Code)
	}
	for _, q := range []string{"?region=USA", "?region=1A", "?x=1", "?region=US&region=GB"} {
		if w := req(s, base+q, "synthetic-test-token"); w.Code != 400 {
			t.Fatalf("%s accepted %d", q, w.Code)
		}
	}
	if w := req(s, base+"?region=GB", "synthetic-test-token"); w.Code != 404 {
		t.Fatalf("unknown region %d", w.Code)
	}
	if calls.Load() != 0 {
		t.Fatal("list reached the provider")
	}
	if !registeredRoute(base) {
		t.Fatal("self-registered credentials cannot read service-top")
	}
}
