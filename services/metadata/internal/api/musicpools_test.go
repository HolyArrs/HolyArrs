package api

import (
	"strings"
	"testing"
)

type fakePools struct{ body, etag string }

func (f fakePools) Pools() ([]byte, string, bool) { return []byte(f.body), f.etag, f.body != "" }

func TestMusicPoolsRoute(t *testing.T) {
	s, _, _ := setup(Options{Burst: 100, Pools: fakePools{`{"version":"abc"}`, `"abc"`}})
	w := req(s, "/v1/music/pools", "")
	cc := w.Header().Get("Cache-Control")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "abc") || !strings.Contains(cc, "public") || !strings.Contains(cc, "s-maxage=21600") || w.Header().Get("ETag") != `"abc"` {
		t.Fatalf("%d %q %q %s", w.Code, cc, w.Header().Get("ETag"), w.Body.String())
	}
	if w := req(s, "/v1/music/pools?key=genre:pop", ""); w.Code != 400 {
		t.Fatalf("a key in the query must be refused (one bundle for everyone): %d", w.Code)
	}
	s2, _, _ := setup(Options{Burst: 100})
	if w := req(s2, "/v1/music/pools", ""); w.Code != 404 {
		t.Fatalf("off: %d", w.Code)
	}
}

func TestMusicPoolsIsSharedEdgeRoute(t *testing.T) {
	if !strings.HasPrefix(edgePolicy("/v1/music/pools"), "public") {
		t.Fatal("pools not public on the edge")
	}
}
