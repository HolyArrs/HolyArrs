package musictags

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/musictrend"
)

// testCharts puts artist ar1 on the us chart (stats artists need real ids).
func testCharts(context.Context) []ChartRow {
	return []ChartRow{{Country: "us", Kind: "albums", Items: []musictrend.ChartItem{{Rank: 2, MBID: "g1", Title: "Debut", Artist: "Band", ArtistMBID: "ar1"}}}}
}

func fakeUpstream(t *testing.T, calls *int64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(calls, 1)
		p := r.URL.Path + "?" + r.URL.RawQuery
		switch {
		case strings.Contains(p, "/stats/sitewide/artists"):
			w.Write([]byte(`{"payload":{"artists":[{"artist_mbid":"ar1","artist_name":"Band","listen_count":10}]}}`))
		case strings.Contains(p, "/stats/sitewide/release-groups"):
			w.Write([]byte(`{"payload":{"release_groups":[]}}`))
		case strings.Contains(p, "/explore/fresh-releases"):
			w.Write([]byte(`{"payload":{"releases":[]}}`))
		case strings.Contains(p, "/popularity/release-group"):
			w.Write([]byte(`[{"release_group_mbid":"g1","total_listen_count":500,"total_user_count":50}]`))
		case strings.Contains(p, "/metadata/release_group"):
			w.Write([]byte(`{"g1":{"release_group":{"caa_id":5,"name":"Debut","date":"1993-02-22","type":"Album"},"tag":{"release_group":[{"count":3,"tag":"rock"}]}}}`))
		case strings.Contains(p, "/release-group"):
			w.Write([]byte(mbGroupsFixture))
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestServiceRefreshServesBundleAndCaches(t *testing.T) {
	var calls int64
	up := fakeUpstream(t, &calls)
	defer up.Close()
	now := fixedNow
	s, err := Open(filepath.Join(t.TempDir(), "musictags.sqlite"), Options{HTTP: up.Client(), MBHTTP: up.Client(), LBRoot: up.URL + "/1", MBRoot: up.URL + "/ws/2", UserAgent: "test", Now: func() time.Time { return now }, Charts: testCharts})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, _, ok := s.Pools(); ok {
		t.Fatal("bundle before the first refresh")
	}
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	body, etag, ok := s.Pools()
	if !ok || etag == "" {
		t.Fatal("no bundle after refresh")
	}
	var b Bundle
	if err := json.Unmarshal(body, &b); err != nil || len(b.Items) == 0 || len(b.Keys["genre:rock"]) == 0 {
		t.Fatalf("bundle = %d items, rock %d, err %v", len(b.Items), len(b.Keys["genre:rock"]), err)
	}
	first := atomic.LoadInt64(&calls)
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt64(&calls); got != first {
		t.Fatalf("second refresh made %d calls, want 0 (cached)", got-first)
	}
	// A new day refetches the daily lists only.
	now = now.Add(25 * time.Hour)
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt64(&calls) - first; got == 0 || got > 12 {
		t.Fatalf("next day made %d calls, want the daily lists only", got)
	}
	// The stored bundle answers after a restart.
	s.Close()
	s2, err := Open(filepath.Join(filepath.Dir(s.path), "musictags.sqlite"), Options{HTTP: up.Client(), MBHTTP: up.Client(), LBRoot: up.URL + "/1", MBRoot: up.URL + "/ws/2", UserAgent: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if _, e2, ok := s2.Pools(); !ok || e2 == "" {
		t.Fatal("bundle lost on restart")
	}
}

func TestServiceCapsMusicBrainzMissesPerRun(t *testing.T) {
	var calls int64
	up := fakeUpstream(t, &calls)
	defer up.Close()
	s, _ := Open(filepath.Join(t.TempDir(), "m.sqlite"), Options{HTTP: up.Client(), MBHTTP: up.Client(), LBRoot: up.URL + "/1", MBRoot: up.URL + "/ws/2", MBPerRun: 0, UserAgent: "t", Now: func() time.Time { return fixedNow }})
	defer s.Close()
	s.o.MBPerRun = -1 // no MusicBrainz calls at all
	_ = s.Refresh(context.Background())
	for _, u := range s.seen {
		if strings.Contains(u, "/ws/2/") {
			t.Fatalf("MusicBrainz called over the cap: %s", u)
		}
	}
}

func TestAlbumsListsTheCatalogAlbumsChartingAndPlayedFirst(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "m.sqlite"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if len(s.Albums()) != 0 {
		t.Fatal("albums without a bundle")
	}
	s.body = []byte(`{"version":"v1","items":[
		{"id":"a1","k":"a","t":"Quiet","ar":"X","l":50},
		{"id":"r1","k":"r","t":"X","l":9999},
		{"id":"a2","k":"a","t":"Loud","ar":"Y","l":900},
		{"id":"a3","k":"a","t":"Hit","ar":"Z","l":10,"c":{"us":3,"gb":5}}]}`)
	s.etag = `"v1"`
	got := s.Albums()
	if len(got) != 3 || got[0].MBID != "a3" || got[1].MBID != "a2" || got[2].MBID != "a1" || got[1].Artist != "Y" || got[1].Title != "Loud" {
		t.Fatalf("albums %+v", got)
	}
	if a := s.Artists(); len(a) != 1 || a[0] != "r1" {
		t.Fatalf("artists %v", a)
	}
}
