package api

import (
	"strings"
	"testing"
)

type fakeShelves map[string]string

func (f fakeShelves) Shelves(region string) ([]byte, string, bool) {
	b, ok := f[region]
	if !ok {
		b, ok = f["global"]
	}
	return []byte(b), `"` + region + `"`, ok
}

func TestMusicShelvesRoute(t *testing.T) {
	s, _, _ := setup(Options{Burst: 100, Shelves: fakeShelves{"global": `{"region":"global"}`, "us": `{"region":"us"}`}})
	w := req(s, "/v1/music/lists/shelves?region=US", "")
	cc := w.Header().Get("Cache-Control")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"us"`) || !strings.Contains(cc, "public") || !strings.Contains(cc, "max-age=3600") {
		t.Fatalf("%d %q %s", w.Code, cc, w.Body.String())
	}
	if w := req(s, "/v1/music/lists/shelves", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "global") {
		t.Fatalf("default region %d", w.Code)
	}
	for _, u := range []string{"/v1/music/lists/shelves?region=usa", "/v1/music/lists/shelves?x=1"} {
		if w := req(s, u, ""); w.Code != 400 {
			t.Fatalf("%s: %d", u, w.Code)
		}
	}
	s2, _, _ := setup(Options{Burst: 100})
	if w := req(s2, "/v1/music/lists/shelves", ""); w.Code != 404 {
		t.Fatalf("off: %d", w.Code)
	}
}
