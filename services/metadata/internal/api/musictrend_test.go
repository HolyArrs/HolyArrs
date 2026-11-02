package api

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CoderLuii/holyarrs-metadata/internal/musictrend"
)

type fakeMusic map[string]string

func (f fakeMusic) List(kind, region string) ([]byte, string, bool) {
	b, ok := f[kind+"|"+region]
	return []byte(b), `"etag-` + kind + region + `"`, ok
}

func TestMusicTrendingRoute(t *testing.T) {
	s, _, calls := setup(Options{Burst: 100, Music: fakeMusic{"artist|global": `{"a":1}`, "album|FR": `{"fr":1}`}})
	const base = "/v1/lists/music-trending"
	if w := req(s, base+"?kind=artist", ""); w.Code == 401 || w.Code == 403 {
		t.Fatalf("shared route refused without a credential %d", w.Code)
	}
	w := req(s, base+"?kind=artist", "synthetic-test-token")
	if w.Code != 200 || w.Body.String() != `{"a":1}` || w.Header().Get("ETag") != `"etag-artistglobal"` || !strings.HasPrefix(w.Header().Get("Cache-Control"), "public, ") || w.Header().Get("Vary") != "" {
		t.Fatalf("list %d %v %s", w.Code, w.Header(), w.Body)
	}
	if w := req(s, base+"?kind=album&region=fr", "synthetic-test-token"); w.Code != 200 || w.Body.String() != `{"fr":1}` {
		t.Fatalf("country %d", w.Code)
	}
	r := httptest.NewRequest("GET", base+"?kind=artist&region=global", nil)
	r.Header.Set("Authorization", "Bearer synthetic-test-token")
	r.Header.Set("If-None-Match", `"etag-artistglobal"`)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, r)
	if rec.Code != 304 || rec.Body.Len() != 0 {
		t.Fatalf("conditional %d", rec.Code)
	}
	for _, q := range []string{"", "?kind=track", "?kind=artist&region=FRA", "?kind=artist&lang=en", "?kind=artist&kind=album", "?kind=album&region=1A"} {
		if w := req(s, base+q, "synthetic-test-token"); w.Code != 400 {
			t.Fatalf("%s accepted %d", q, w.Code)
		}
	}
	if w := req(s, base+"?kind=artist&region=DE", "synthetic-test-token"); w.Code != 404 {
		t.Fatalf("unknown region %d", w.Code)
	}
	if calls.Load() != 0 {
		t.Fatal("list reached the provider")
	}
	s2, _, _ := setup(Options{})
	if w := req(s2, base+"?kind=artist", "synthetic-test-token"); w.Code != 404 {
		t.Fatalf("without lists %d", w.Code)
	}
}

func TestMusicTrendingForRegisteredCredential(t *testing.T) {
	s, _ := registering(Options{Registration: true, Burst: 100, TrustedProxies: testProxy(), Music: fakeMusic{"artist|global": `{}`}})
	token := registerToken(t, s, "203.0.113.40")
	if w := reqFrom(s, "/v1/lists/music-trending?kind=artist", token, "203.0.113.40"); w.Code != 200 {
		t.Fatalf("registered credential refused %d", w.Code)
	}
}

type fakeSimilar map[string]string

func (f fakeSimilar) Artists(_ context.Context, mbid string) ([]byte, string, error) {
	if b, ok := f[mbid]; ok {
		return []byte(b), "hit", nil
	}
	if mbid == similarDown {
		return nil, "miss", errors.New("source down")
	}
	return nil, "miss", musictrend.ErrNoSimilar
}

const (
	similarRef  = "a74b1b7f-71a5-4011-9441-d0b5e4122711"
	similarDown = "11111111-1111-4111-8111-111111111111"
)

func TestMusicSimilarRoute(t *testing.T) {
	s, _, calls := setup(Options{Burst: 100, Similar: fakeSimilar{similarRef: `{"artists":[]}`}})
	base := "/v1/music/similar/"
	if w := req(s, base+similarRef, ""); w.Code == 401 || w.Code == 403 {
		t.Fatalf("shared route refused without a credential %d", w.Code)
	}
	if w := req(s, base+strings.ToUpper(similarRef), "synthetic-test-token"); w.Code != 200 || w.Body.String() != `{"artists":[]}` || !strings.HasPrefix(w.Header().Get("Cache-Control"), "public, ") || w.Header().Get("X-Metadata-Cache") != "hit" {
		t.Fatalf("similar %d %v %s", w.Code, w.Header(), w.Body)
	}
	for _, p := range []string{"nope", similarRef + "/x", similarRef + "?x=1", ""} {
		if w := req(s, base+p, "synthetic-test-token"); w.Code != 400 {
			t.Fatalf("%q accepted %d", p, w.Code)
		}
	}
	if w := req(s, base+"22222222-2222-4222-8222-222222222222", "synthetic-test-token"); w.Code != 404 {
		t.Fatalf("unknown artist %d", w.Code)
	}
	if w := req(s, base+similarDown, "synthetic-test-token"); w.Code != 503 {
		t.Fatalf("source down %d", w.Code)
	}
	if calls.Load() != 0 {
		t.Fatal("similar reached the provider")
	}
	s2, _, _ := setup(Options{})
	if w := req(s2, base+similarRef, "synthetic-test-token"); w.Code != 404 {
		t.Fatalf("without a source %d", w.Code)
	}
	reg, _ := registering(Options{Registration: true, Burst: 100, TrustedProxies: testProxy(), Similar: fakeSimilar{similarRef: `{}`}})
	token := registerToken(t, reg, "203.0.113.41")
	if w := reqFrom(reg, base+similarRef, token, "203.0.113.41"); w.Code != 200 {
		t.Fatalf("registered credential refused %d", w.Code)
	}
}

func TestMusicTrendingExtraKinds(t *testing.T) {
	s, _, _ := setup(Options{Burst: 100, Music: fakeMusic{"fresh|global": `{"f":1}`, "classic|global": `{"c":1}`, "rising|global": `{"r":1}`}})
	for _, k := range []string{"fresh", "classic", "rising"} {
		if w := req(s, "/v1/lists/music-trending?kind="+k, "synthetic-test-token"); w.Code != 200 {
			t.Fatalf("%s %d", k, w.Code)
		}
		if w := req(s, "/v1/lists/music-trending?kind="+k+"&region=FR", "synthetic-test-token"); w.Code != 400 {
			t.Fatalf("%s by country %d", k, w.Code)
		}
	}
}

type fakeCovers struct{ artist, title string }

func (f *fakeCovers) Album(_ context.Context, mbid, artist, title string) ([]byte, string, error) {
	f.artist, f.title = artist, title
	switch mbid {
	case similarRef:
		return []byte(`{"url":"https://x/c.jpg"}`), "miss", nil
	case similarDown:
		return nil, "miss", errors.New("source down")
	}
	return nil, "miss", ErrNoCover
}

func TestMusicCoverRoute(t *testing.T) {
	f := &fakeCovers{}
	s, _, calls := setup(Options{Burst: 100, Covers: f})
	base := "/v1/music/cover/"
	if w := req(s, base+similarRef, ""); w.Code == 401 || w.Code == 403 {
		t.Fatalf("shared route refused without a credential %d", w.Code)
	}
	if w := req(s, base+similarRef+"?artist=Taylor+Swift&title=The+Encore", "synthetic-test-token"); w.Code != 200 || w.Body.String() != `{"url":"https://x/c.jpg"}` || w.Header().Get("X-Metadata-Cache") != "miss" || f.artist != "Taylor Swift" || f.title != "The Encore" {
		t.Fatalf("cover %d %v %s %+v", w.Code, w.Header(), w.Body, f)
	}
	for _, p := range []string{"nope", similarRef + "?x=1", similarRef + "?artist=a&artist=b", similarRef + "?title=" + strings.Repeat("a", 301), ""} {
		if w := req(s, base+p, "synthetic-test-token"); w.Code != 400 {
			t.Fatalf("%q accepted %d", p, w.Code)
		}
	}
	if w := req(s, base+"22222222-2222-4222-8222-222222222222", "synthetic-test-token"); w.Code != 404 {
		t.Fatalf("no cover %d", w.Code)
	}
	if w := req(s, base+similarDown, "synthetic-test-token"); w.Code != 503 {
		t.Fatalf("source down %d", w.Code)
	}
	if calls.Load() != 0 {
		t.Fatal("cover reached the provider")
	}
	s2, _, _ := setup(Options{})
	if w := req(s2, base+similarRef, "synthetic-test-token"); w.Code != 404 {
		t.Fatalf("without a source %d", w.Code)
	}
}
