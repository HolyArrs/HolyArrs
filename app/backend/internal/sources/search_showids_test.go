package sources

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A show search sends every id the Source announces for tv-search, and none
// it does not.
func TestShowSearchSendsAnnouncedIDs(t *testing.T) {
	ti := mediafiles.Title{Name: "Lab Show", SourceIDs: map[string]string{"tvdb": "11", "tvmaze": "22", "imdb": "tt0333", "tmdb": "44"}}
	q := showQuery(ti, 2, nil)
	if q.IMDb != "tt0333" || q.TMDb != 44 {
		t.Fatalf("showQuery ids = %q %d", q.IMDb, q.TMDb)
	}
	all := Caps{TVSearch: true, Params: map[string][]string{"tv": {"q", "season", "ep", "tvdbid", "tvmazeid", "imdbid", "tmdbid"}}}
	only := Caps{TVSearch: true, Params: map[string][]string{"tv": {"q", "season", "ep", "tvdbid", "tvmazeid"}}}
	none := Caps{TVSearch: true, Params: map[string][]string{"tv": {"q", "season", "ep"}}}
	episode := q
	episode.Season, episode.Episode = 2, "5"
	for _, c := range []struct {
		name string
		q    Query
		caps Caps
		want map[string]string
	}{
		{"season, all ids", q, all, map[string]string{"tvdbid": "11", "tvmazeid": "22", "imdbid": "0333", "tmdbid": "44", "season": "2", "q": ""}},
		{"episode, all ids", episode, all, map[string]string{"tvdbid": "11", "tvmazeid": "22", "imdbid": "0333", "tmdbid": "44", "season": "2", "ep": "5"}},
		{"episode, tvdb and tvmaze only", episode, only, map[string]string{"tvdbid": "11", "tvmazeid": "22", "imdbid": "", "tmdbid": "", "ep": "5"}},
		{"episode, no ids", episode, none, map[string]string{"tvdbid": "", "tvmazeid": "", "imdbid": "", "tmdbid": "", "q": "Lab Show", "ep": "5"}},
	} {
		v, ids, err := searchValues(c.q, c.caps, true)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		for k, w := range c.want {
			if v.Get(k) != w {
				t.Errorf("%s: %s = %q, want %q (%v)", c.name, k, v.Get(k), w, v.Encode())
			}
		}
		if ids != (c.want["tvdbid"] != "") {
			t.Errorf("%s: ids = %v", c.name, ids)
		}
		// The title search never carries an id.
		v, _, _ = searchValues(c.q, c.caps, false)
		for _, k := range []string{"tvdbid", "tvmazeid", "imdbid", "tmdbid"} {
			if v.Has(k) {
				t.Errorf("%s: title search sent %s", c.name, k)
			}
		}
	}
	// A show known only by IMDb still searches by id.
	v, ids, err := searchValues(Query{Title: "Lab Show", TV: true, IMDb: "tt0333"}, all, true)
	if err != nil || !ids || v.Get("imdbid") != "0333" || v.Has("q") {
		t.Fatalf("imdb only: %v %v %v", v.Encode(), ids, err)
	}
}

// An id search that finds nothing at all asks once more by title: a Source
// may need every id it is sent to match, and one outdated id finds nothing.
func TestSearch_EmptyIDAnswerSearchesByTitle(t *testing.T) {
	for _, c := range []struct {
		name, caps string
		q          Query
	}{
		{"show", `<tv-search available="yes" supportedParams="q,season,ep,tvdbid,imdbid"/>`, Query{Title: "Lab Show", TV: true, Season: 1, TVDB: 11, IMDb: "tt0333"}},
		{"movie", `<movie-search available="yes" supportedParams="q,imdbid,tmdbid"/>`, Query{Title: "Lab Movie", Year: 2020, IMDb: "tt0444", TMDb: 55}},
	} {
		byID, byTitle := 0, 0
		srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
			v := r.URL.Query()
			switch {
			case v.Get("t") == "caps":
				_, _ = w.Write([]byte(`<caps><searching><search available="yes" supportedParams="q"/>` + c.caps + `</searching></caps>`))
			case v.Get("q") != "":
				byTitle++
				_, _ = w.Write([]byte(tvRSS(strings.ReplaceAll(c.q.Title, " ", ".") + ".S01.1080p.WEB-GRP")))
			default:
				byID++
				_, _ = w.Write([]byte(tvRSS()))
			}
		})
		items, err := New(Config{URL: srv.URL + "/api", Protocol: "torrent", AllowLocal: true}).Search(context.Background(), c.q)
		if err != nil || len(items) != 1 {
			t.Fatalf("%s: items %v err %v", c.name, titles(items), err)
		}
		if byID != 1 || byTitle != 1 {
			t.Fatalf("%s: id searches %d, title searches %d", c.name, byID, byTitle)
		}
	}
}

// The IMDb id is sent without its prefix, whatever its case or spacing.
func TestSearchValues_IMDbNormalised(t *testing.T) {
	caps := Caps{TVSearch: true, MovieSearch: true, Params: map[string][]string{"tv": {"q", "imdbid"}, "movie": {"q", "imdbid"}}}
	for _, in := range []struct{ id, want string }{{"TT0123456", "0123456"}, {" tt1 ", "1"}, {"0999", "0999"}} {
		for _, tv := range []bool{true, false} {
			v, ids, err := searchValues(Query{Title: "Lab", TV: tv, IMDb: in.id}, caps, true)
			if err != nil || !ids || v.Get("imdbid") != in.want {
				t.Errorf("tv=%v %q: imdbid %q ids %v err %v", tv, in.id, v.Get("imdbid"), ids, err)
			}
		}
	}
}
