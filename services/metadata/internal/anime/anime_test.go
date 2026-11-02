package anime

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Synthetic lists in the shape of the two community files.
const masterXML = `<?xml version="1.0" encoding="utf-8"?>
<anime-list>
  <anime anidbid="100" tvdbid="5000" defaulttvdbseason="1" episodeoffset="" tmdbtv="9000" tmdbseason="1" tmdboffset="" tmdbid="" imdbid="tt0000100">
    <name>Synthetic Show</name>
    <mapping-list>
      <mapping anidbseason="0" tvdbseason="0" start="1" end="3" offset="1">;4-0;</mapping>
    </mapping-list>
  </anime>
  <anime anidbid="101" tvdbid="5000" defaulttvdbseason="1" episodeoffset="12" tmdbtv="9000" tmdbseason="2" tmdboffset="" tmdbid="" imdbid="">
    <name>Synthetic Show Part 2</name>
    <mapping-list>
      <mapping anidbseason="1" tvdbseason="1">;1-13+14;</mapping>
    </mapping-list>
  </anime>
  <anime anidbid="102" tvdbid="movie" defaulttvdbseason="" episodeoffset="" tmdbtv="" tmdbseason="" tmdboffset="" tmdbid="7000,7001" imdbid="tt1,tt2">
    <name>Synthetic Film</name>
  </anime>
  <anime anidbid="103" tvdbid="6000" defaulttvdbseason="a" episodeoffset="" tmdbtv="9100" tmdbseason="a" tmdboffset="" tmdbid="" imdbid="">
    <name>Long Runner</name>
  </anime>
</anime-list>`

const fullJSON = `[
 {"type":"TV","anidb_id":100,"anilist_id":11,"mal_id":21,"kitsu_id":31,"imdb_id":["tt0000100"],"themoviedb_id":{"tv":9000},"tvdb_id":5000,"season":{"tvdb":1,"tmdb":1}},
 {"type":"TV","anidb_id":101,"anilist_id":12,"mal_id":22,"themoviedb_id":{"tv":9000},"tvdb_id":5000,"season":{"tvdb":1,"tmdb":2},"episode_offset":{"tvdb":12}},
 {"type":"MOVIE","anidb_id":102,"anilist_id":13,"mal_id":23,"themoviedb_id":{"movie":[7000]}},
 {"type":"TV","anilist_id":14,"mal_id":24,"themoviedb_id":{"tv":9200},"tvdb_id":6200,"season":{"tvdb":1,"tmdb":1}},
 {"type":"ONA","anilist_id":15,"mal_id":25},
 {"type":"TV","anidb_id":104,"anilist_id":16,"themoviedb_id":{"tv":9300},"season":{"tmdb":3},"episode_offset":{"tmdb":24}}
]`

func TestMerge(t *testing.T) {
	es, err := Merge([]byte(masterXML), []byte(fullJSON))
	if err != nil {
		t.Fatal(err)
	}
	by := map[int64]Entry{}
	for _, e := range es {
		by[e.AniList] = e
	}
	if len(es) != 6 {
		t.Fatalf("entries %d", len(es))
	}
	s1 := by[11]
	if s1.AniDB != 100 || s1.MAL != 21 || s1.Kitsu != 31 || s1.TMDBTV != 9000 || *s1.TMDBSeason != 1 || s1.Type != "TV" || s1.IMDb != "tt0000100" {
		t.Fatalf("s1 %+v", s1)
	}
	if len(s1.Episodes) != 1 || s1.Episodes[0].Start != 1 || s1.Episodes[0].End != 3 || s1.Episodes[0].Offset != 1 || s1.Episodes[0].Pairs[0].AniDB != 4 || s1.Episodes[0].Pairs[0].To[0] != 0 {
		t.Fatalf("s1 episodes %+v", s1.Episodes)
	}
	s2 := by[12]
	if *s2.TMDBSeason != 2 || *s2.TVDBSeason != 1 || s2.TVDBOffset != 12 || len(s2.Episodes[0].Pairs[0].To) != 2 {
		t.Fatalf("s2 %+v", s2)
	}
	film := by[13]
	if film.TVDB != 0 || len(film.TMDBMovies) != 2 || film.TMDBMovies[1] != 7001 || film.IMDb != "tt1" {
		t.Fatalf("film %+v", film)
	}
	if long := es[3]; long.AniDB != 103 || !long.Absolute || long.TMDBSeason != nil {
		t.Fatalf("absolute %+v", long)
	}
	// Fribb-only rows: kept with a TMDB/TVDB ID, dropped without.
	if only := by[14]; only.AniDB != 0 || only.TMDBTV != 9200 || only.TVDB != 6200 {
		t.Fatalf("fribb only %+v", only)
	}
	if _, ok := by[15]; ok {
		t.Fatal("row without TMDB/TVDB kept")
	}
	if n := by[16]; n.AniDB != 104 || *n.TMDBSeason != 3 || n.TMDBOffset != 24 {
		t.Fatalf("fribb anidb %+v", n)
	}
	if _, err := Merge([]byte("<x"), []byte(fullJSON)); err == nil {
		t.Fatal("bad xml")
	}
	if _, err := Merge([]byte(masterXML), []byte("{")); err == nil {
		t.Fatal("bad json")
	}
}

func lists(t *testing.T, fail *atomic.Bool, anilistCalls *atomic.Int64) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/master.xml":
			if fail.Load() {
				w.WriteHeader(500)
				return
			}
			io.WriteString(w, masterXML)
		case "/full.json":
			io.WriteString(w, fullJSON)
		case "/graphql":
			anilistCalls.Add(1)
			if r.Method != "POST" {
				w.WriteHeader(405)
				return
			}
			var q struct {
				Variables struct {
					IDs []int64 `json:"ids"`
				} `json:"variables"`
			}
			json.NewDecoder(r.Body).Decode(&q)
			type title struct {
				Romaji  string `json:"romaji"`
				English string `json:"english"`
				Native  string `json:"native"`
			}
			var media []map[string]any
			for _, id := range q.Variables.IDs {
				if id == 11 {
					media = append(media, map[string]any{"id": id, "title": title{"Gousei Shou", "Synthetic Show", "合成ショー"}})
				}
			}
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"Page": map[string]any{"media": media}}})
		default:
			w.WriteHeader(404)
		}
	}))
}

func TestServiceLookup(t *testing.T) {
	var fail atomic.Bool
	var calls atomic.Int64
	ts := lists(t, &fail, &calls)
	defer ts.Close()
	now := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	s, err := Open(filepath.Join(t.TempDir(), "anime.sqlite"), Options{MasterURL: ts.URL + "/master.xml", FullURL: ts.URL + "/full.json", AniListURL: ts.URL + "/graphql", Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if _, err := s.Lookup(ctx, KeyTMDBTV, 9000); !errors.Is(err, ErrNoData) {
		t.Fatalf("before load %v", err)
	}
	if s.due(ctx) != 0 {
		t.Fatal("empty database not due")
	}
	if err := s.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if s.due(ctx) != 24*time.Hour {
		t.Fatalf("due %v", s.due(ctx))
	}
	a, err := s.Lookup(ctx, KeyTMDBTV, 9000)
	if err != nil || len(a.Entries) != 2 || *a.Entries[0].TMDBSeason != 1 || *a.Entries[1].TMDBSeason != 2 || a.UpdatedAt != "2026-10-21T00:00:00Z" {
		t.Fatalf("tmdb %+v %v", a, err)
	}
	if len(a.Titles) != 1 || a.Titles[0].AniList != 11 || a.Titles[0].Romaji != "Gousei Shou" || a.Titles[0].Native != "合成ショー" {
		t.Fatalf("titles %+v", a.Titles)
	}
	if calls.Load() != 1 {
		t.Fatalf("anilist calls %d", calls.Load())
	}
	// Cached, including the ID AniList did not know.
	if _, err := s.Lookup(ctx, KeyTVDB, 5000); err != nil || calls.Load() != 1 {
		t.Fatalf("cache %v %d", err, calls.Load())
	}
	// AniList and MAL hits widen to the whole show.
	for _, k := range []struct {
		kind string
		id   int64
	}{{KeyAniList, 12}, {KeyMAL, 21}} {
		if a, err := s.Lookup(ctx, k.kind, k.id); err != nil || len(a.Entries) != 2 {
			t.Fatalf("%s %+v %v", k.kind, a, err)
		}
	}
	if a, err := s.Lookup(ctx, KeyTMDBMovie, 7001); err != nil || len(a.Entries) != 1 || a.Entries[0].AniDB != 102 {
		t.Fatalf("movie %+v %v", a, err)
	}
	if _, err := s.Lookup(ctx, KeyTMDBTV, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("miss %v", err)
	}
	// Derived mappings only: no list names in the answer.
	b, _ := json.Marshal(a)
	if strings.Contains(string(b), "Synthetic Show Part 2") {
		t.Fatal("list name exposed")
	}
	// A failed download keeps the old tables.
	fail.Store(true)
	if err := s.Refresh(ctx); err == nil {
		t.Fatal("refresh should fail")
	}
	if st := s.Stats(); st.Entries != 6 || st.LastError == "" {
		t.Fatalf("stats %+v", st)
	}
	if _, err := s.Lookup(ctx, KeyTMDBTV, 9000); err != nil {
		t.Fatalf("old data lost %v", err)
	}
}

func TestTitlesFailSoft(t *testing.T) {
	var fail atomic.Bool
	var calls atomic.Int64
	ts := lists(t, &fail, &calls)
	defer ts.Close()
	s, err := Open(filepath.Join(t.TempDir(), "anime.sqlite"), Options{MasterURL: ts.URL + "/master.xml", FullURL: ts.URL + "/full.json", AniListURL: ts.URL + "/missing"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	a, err := s.Lookup(context.Background(), KeyTMDBTV, 9000)
	if err != nil || len(a.Entries) != 2 || a.Titles == nil || len(a.Titles) != 0 {
		t.Fatalf("fail soft %+v %v", a, err)
	}
}
