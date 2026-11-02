package musictags

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/musictrend"
)

var fixedNow = time.Date(2026, 10, 24, 6, 30, 0, 0, time.UTC)

// fakeFetch answers URLs by their longest matching key.
type fakeFetch struct {
	m     map[string]string
	calls []string
}

func (f *fakeFetch) answer(u string) ([]byte, error) {
	f.calls = append(f.calls, u)
	best := ""
	for k := range f.m {
		if strings.Contains(u, k) && len(k) > len(best) {
			best = k
		}
	}
	if best == "" {
		return nil, errors.New("no fixture for " + u)
	}
	return []byte(f.m[best]), nil
}

func (f *fakeFetch) Get(_ context.Context, u string) ([]byte, error) { return f.answer(u) }
func (f *fakeFetch) Post(_ context.Context, u string, _ []byte) ([]byte, error) {
	return f.answer(u)
}

func TestCollectBuildsCatalogFromSources(t *testing.T) {
	lb := &fakeFetch{m: map[string]string{
		"/stats/sitewide/artists":        `{"payload":{"artists":[{"artist_mbid":"ar1","artist_name":"Band","listen_count":10}]}}`,
		"/stats/sitewide/release-groups": `{"payload":{"release_groups":[]}}`,
		"/explore/fresh-releases":        `{"payload":{"releases":[]}}`,
		"/popularity/release-group":      `[{"release_group_mbid":"g1","total_listen_count":500,"total_user_count":50},{"release_group_mbid":"g4","total_listen_count":20,"total_user_count":9}]`,
		"/metadata/release_group":        `{"g1":{"release_group":{"caa_id":5,"name":"Debut","date":"1993-02-22","type":"Album"},"tag":{"artist":[{"artist_mbid":"ar1","count":9,"tag":"rock"}],"release_group":[{"count":3,"tag":"rock"}]}},"g4":{"release_group":{"caa_id":6,"name":"Short","type":"EP"},"tag":{}}}`,
	}}
	mb := &fakeFetch{m: map[string]string{"/release-group?artist=ar1": mbGroupsFixture}}
	charts := func(context.Context) []ChartRow {
		return []ChartRow{{Country: "us", Kind: "albums", Items: []musictrend.ChartItem{{Rank: 2, MBID: "g1", Title: "Debut", Artist: "Band", ArtistMBID: "ar1", Genres: []string{"Rock"}}}}}
	}
	src, err := Collect(context.Background(), lb, mb, Config{LB: "https://lb/1", MB: "https://mb/ws/2", Keep: 10, Charts: charts})
	if err != nil {
		t.Fatal(err)
	}
	rs := []Recipe{{Key: "genre:rock", Name: "Rock", Kind: KindGenre, Tags: map[string]float64{"rock": 1}}}
	b := Build(src, rs, fixedNow)
	var g1 *BItem
	for i := range b.Items {
		if b.Items[i].ID == "g1" {
			g1 = &b.Items[i]
		}
	}
	if g1 == nil || g1.Listens != 500 || g1.Users != 50 || !g1.Debut || g1.C["us"] != 2 || g1.Artist != "Band" {
		t.Fatalf("g1 = %+v", g1)
	}
	if len(b.Keys["genre:rock"]) == 0 {
		t.Fatal("rock pool empty")
	}
}

func TestCollectSearchesTagsForSmallKeys(t *testing.T) {
	lb := &fakeFetch{m: map[string]string{
		"/stats/sitewide/artists":        `{"payload":{"artists":[]}}`,
		"/stats/sitewide/release-groups": `{"payload":{"release_groups":[]}}`,
		"/explore/fresh-releases":        `{"payload":{"releases":[]}}`,
		"/popularity/release-group":      `[{"release_group_mbid":"s1","total_listen_count":900,"total_user_count":80}]`,
		"/metadata/release_group":        `{"s1":{"release_group":{"caa_id":7,"name":"Blue Hour","date":"1962","type":"Album"},"tag":{"release_group":[{"count":4,"tag":"blues"}]}}}`,
	}}
	mb := &fakeFetch{m: map[string]string{"/release-group?query=": `{"release-groups":[{"id":"S1","title":"Blue Hour","primary-type":"Album","secondary-types":[],"first-release-date":"1962","artist-credit":[{"name":"Singer","artist":{"id":"AR5","name":"Singer"}}]},{"id":"S2","title":"Live Blues","primary-type":"Album","secondary-types":["Live"],"artist-credit":[{"artist":{"id":"AR6","name":"Other"}}]}]}`}}
	cfg := Config{LB: "https://lb/1", MB: "https://mb/ws/2", Keep: 10, SearchTags: func(*Source) []string { return []string{"blues"} }}
	src, err := Collect(context.Background(), lb, mb, cfg)
	if err != nil {
		t.Fatal(err)
	}
	b := Build(src, []Recipe{{Key: "genre:blues", Name: "Blues", Kind: KindGenre, Tags: map[string]float64{"blues": 1}}}, fixedNow)
	found := false
	for _, it := range b.Items {
		if it.ID == "s1" && it.Artist == "Singer" && it.ArtistID == "ar5" && it.Listens == 900 {
			found = true
		}
		if it.ID == "s2" {
			t.Fatal("live album from the search kept")
		}
	}
	if !found || len(b.Keys["genre:blues"]) == 0 {
		t.Fatalf("searched album missing: %+v", b.Items)
	}
	searched := false
	for _, u := range mb.calls {
		searched = searched || strings.Contains(u, "query=tag%3A%22blues%22")
	}
	if !searched {
		t.Fatalf("no tag search in %v", mb.calls)
	}
}

func TestCollectAddsShelfItems(t *testing.T) {
	lb := &fakeFetch{m: map[string]string{
		"/stats/sitewide/artists":        `{"payload":{"artists":[]}}`,
		"/stats/sitewide/release-groups": `{"payload":{"release_groups":[]}}`,
		"/explore/fresh-releases":        `{"payload":{"releases":[]}}`,
		"/popularity/release-group":      `[{"release_group_mbid":"sh1","total_listen_count":900,"total_user_count":80}]`,
		"/metadata/release_group":        `{"sh1":{"release_group":{"name":"Shelf Album","date":"2026-10-10","type":"Album"},"tag":{"release_group":[{"count":2,"tag":"pop"}]}}}`,
	}}
	mb := &fakeFetch{m: map[string]string{}}
	cfg := Config{LB: "https://lb/1", MB: "https://mb/ws/2", Shelves: func(context.Context) []ShelfItem {
		return []ShelfItem{{Kind: "album", MBID: "SH1", ArtistMBID: "AR7", Title: "Shelf Album", Artist: "Star"}, {Kind: "artist", MBID: "AR8", Artist: "Other"}}
	}}
	src, err := Collect(context.Background(), lb, mb, cfg)
	if err != nil {
		t.Fatal(err)
	}
	b := Build(src, []Recipe{{Key: "genre:pop", Name: "Pop", Kind: KindGenre, Tags: map[string]float64{"pop": 1}}}, fixedNow)
	ids := map[string]bool{}
	for _, it := range b.Items {
		ids[it.Kind+":"+it.ID] = true
	}
	if !ids["a:sh1"] || !ids["r:ar8"] {
		t.Fatalf("shelf items missing: %v", ids)
	}
}

func TestSmallKeyTags(t *testing.T) {
	src := NewSource()
	for i := 0; i < 5; i++ {
		src.Album(Album{ID: string(rune('a'+i)) + "x", Title: "T", Tags: tags("rock", 3), Listens: 1000, Art: true})
	}
	rs := []Recipe{
		{Key: "genre:rock", Name: "Rock", Kind: KindGenre, Tags: map[string]float64{"rock": 1}},
		{Key: "genre:blues", Name: "Blues", Kind: KindGenre, Tags: map[string]float64{"blues": 1, "delta blues": 1, "chicago blues": 1, "electric blues": 1, "blues rock": 0.8}},
		{Key: "decade:1990s", Name: "90s", Kind: KindDecade, From: 1990, To: 1999},
	}
	got := SmallKeyTags(src, rs, fixedNow, 3, 2)
	if len(got) != 2 || got[0] != "blues" {
		t.Fatalf("tags = %v, want the 2 first core blues tags (rock has 5 albums, decades never)", got)
	}
}

func TestCollectSkipsArtistsWhenMusicBrainzRefuses(t *testing.T) {
	lb := &fakeFetch{m: map[string]string{
		"/stats/sitewide/artists":        `{"payload":{"artists":[{"artist_mbid":"ar1","artist_name":"Band","listen_count":10}]}}`,
		"/stats/sitewide/release-groups": `{"payload":{"release_groups":[]}}`,
		"/explore/fresh-releases":        `{"payload":{"releases":[]}}`,
		"/popularity/release-group":      `[]`,
		"/metadata/release_group":        `{}`,
	}}
	mb := &fakeFetch{m: map[string]string{}}
	if _, err := Collect(context.Background(), lb, mb, Config{LB: "https://lb/1", MB: "https://mb/ws/2", Keep: 10}); err != nil {
		t.Fatalf("a refused artist must not fail the run: %v", err)
	}
}
