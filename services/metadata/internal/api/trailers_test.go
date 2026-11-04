package api

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoderLuii/holyarrs-metadata/internal/stats"
	"github.com/CoderLuii/holyarrs-metadata/internal/trailers"
)

type fakeTrailers struct{ asked, who []string }

func (f *fakeTrailers) Pick(_ context.Context, kind string, id int64, lang, who string) (trailers.Pick, error) {
	f.who = append(f.who, who)
	f.asked = append(f.asked, kind+"|"+lang)
	if id == 404 {
		return trailers.Pick{}, trailers.ErrNoTrailer
	}
	return trailers.Pick{Key: "trailerKey1", Name: "Official Trailer", Language: lang, Status: "ok", CheckedAt: "2026-10-19T00:00:00Z"}, nil
}

func (f *fakeTrailers) Stats() trailers.Stats {
	return trailers.Stats{Checked: 7, Broken: 2, Replaced: 1, LastRun: "2026-10-19T00:00:00Z"}
}

func TestTrailerRoute(t *testing.T) {
	ft := &fakeTrailers{}
	s, st := registering(Options{Registration: true, Burst: 100, Trailers: ft, TrustedProxies: testProxy()})
	if w := req(s, "/v1/trailers/movie/11", ""); w.Code != 401 {
		t.Fatalf("no credential %d", w.Code)
	}
	w := req(s, "/v1/trailers/movie/11?language=de", "synthetic-test-token")
	var p trailers.Pick
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &p) != nil || p.Key != "trailerKey1" || p.Language != "de" || p.Status != "ok" {
		t.Fatalf("pick %d %s", w.Code, w.Body)
	}
	// Self-registered credentials reach it too (lookup scope).
	tok := registerToken(t, s, "203.0.113.40")
	if st.sources[tok] == "" {
		t.Fatal("not registered")
	}
	if w := reqFrom(s, "/v1/trailers/tv/12", tok, "203.0.113.40"); w.Code != 200 {
		t.Fatalf("registered %d", w.Code)
	}
	// Popularity is counted per network for registered credentials, per
	// credential digest for operator ones; never the raw credential.
	if ft.who[len(ft.who)-1] != "net:203.0.113.0/24" || !strings.HasPrefix(ft.who[0], "token:") || strings.Contains(ft.who[0], "synthetic") {
		t.Fatalf("asker %v", ft.who)
	}
	if ft.asked[len(ft.asked)-1] != "tv|en" {
		t.Fatalf("default language %v", ft.asked)
	}
	if w := req(s, "/v1/trailers/movie/404", "synthetic-test-token"); w.Code != 404 || !strings.Contains(w.Body.String(), "no_trailer") {
		t.Fatalf("none %d", w.Code)
	}
	for _, q := range []string{"/v1/trailers/show/1", "/v1/trailers/movie/0", "/v1/trailers/movie/01", "/v1/trailers/movie/x", "/v1/trailers/movie/1/2", "/v1/trailers/movie/1?language=deu", "/v1/trailers/movie/1?x=1", "/v1/trailers/movie"} {
		if w := req(s, q, "synthetic-test-token"); w.Code != 400 {
			t.Fatalf("%s accepted %d", q, w.Code)
		}
	}
	s2, _, _ := setup(Options{Burst: 100})
	if w := req(s2, "/v1/trailers/movie/11", "synthetic-test-token"); w.Code != 404 {
		t.Fatalf("off %d", w.Code)
	}
}

func TestStatsCarryTrailers(t *testing.T) {
	rec, err := stats.Open(filepath.Join(t.TempDir(), "stats.sqlite"), stats.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Close()
	s, _, _ := setup(Options{Burst: 100, Stats: rec, StatsToken: syntheticReader, Trailers: &fakeTrailers{}})
	w := req(s, "/v1/stats", syntheticReader)
	var b struct {
		Trailers *trailers.Stats `json:"trailers"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &b) != nil || b.Trailers == nil || b.Trailers.Checked != 7 || b.Trailers.Broken != 2 || b.Trailers.Replaced != 1 {
		t.Fatalf("stats %d %s", w.Code, w.Body)
	}
}
