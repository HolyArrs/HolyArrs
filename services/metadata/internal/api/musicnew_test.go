package api

import "testing"

type fakeNew map[string]string

func (f fakeNew) Window(w string) ([]byte, string, bool) {
	b, ok := f[w]
	return []byte(b), `"etag-` + w + `"`, ok
}

func TestMusicNewRoute(t *testing.T) {
	s, _, calls := setup(Options{Burst: 100, NewMusic: fakeNew{"week": `{"w":1}`, "upcoming": `{"u":1}`}})
	const base = "/v1/music/new"
	if w := req(s, base, ""); w.Code == 401 || w.Code == 403 {
		t.Fatalf("shared route refused without a credential %d", w.Code)
	}
	if w := req(s, base, "synthetic-test-token"); w.Code != 200 || w.Body.String() != `{"w":1}` || w.Header().Get("ETag") != `"etag-week"` {
		t.Fatalf("default window %d %s", w.Code, w.Body)
	}
	if w := req(s, base+"?window=upcoming", "synthetic-test-token"); w.Code != 200 || w.Body.String() != `{"u":1}` {
		t.Fatalf("upcoming %d", w.Code)
	}
	if w := req(s, base+"?window=tracks", "synthetic-test-token"); w.Code != 404 {
		t.Fatalf("missing window %d", w.Code)
	}
	for _, q := range []string{"?window=year", "?window=week&window=recent", "?kind=album"} {
		if w := req(s, base+q, "synthetic-test-token"); w.Code != 400 {
			t.Fatalf("%s accepted %d", q, w.Code)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("reached the provider")
	}
	s2, _, _ := setup(Options{})
	if w := req(s2, base, "synthetic-test-token"); w.Code != 404 {
		t.Fatalf("without source %d", w.Code)
	}
}
