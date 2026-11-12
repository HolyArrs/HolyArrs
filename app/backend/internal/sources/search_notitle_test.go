package sources

import (
	"context"
	"net/http"
	"net/url"
	"sync"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// capsSite is a fake Source with the given tv-search params. ignoreIDs makes
// it answer an id-only search with its newest releases, as a site that does
// not know the id does; a text search returns what matches the words.
type capsSite struct {
	mu        sync.Mutex
	queries   []url.Values
	tvParams  string
	ignoreIDs bool
}

var notitleLatest = []string{"Alicias.Law.S01.MULTI.1080p.WEB.H264-GRP", "koebenhavneriet.s01e11.danish.720p.WEB-GRP"}
var notitleHits = []string{"Cabin.in.the.Woods.S01.1080p.AMZN.WEB-DL.DDP2.0.H.264-Kitsune", "The.Cabin.in.the.Woods.2011.1080p.BluRay.x264-GRP"}

func (s *capsSite) start(t *testing.T) *Client {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("t") == "caps" {
			_, _ = w.Write([]byte(`<caps><searching><search available="yes" supportedParams="q"/><tv-search available="yes" supportedParams="` + s.tvParams + `"/></searching></caps>`))
			return
		}
		s.mu.Lock()
		s.queries = append(s.queries, q)
		s.mu.Unlock()
		switch {
		case q.Get("q") != "":
			_, _ = w.Write([]byte(tvRSS(notitleHits...)))
		case s.ignoreIDs:
			_, _ = w.Write([]byte(tvRSS(notitleLatest...)))
		default:
			_, _ = w.Write([]byte(tvRSS(notitleHits[0])))
		}
	})
	return New(Config{URL: srv.URL + "/api", Protocol: "torrent", AllowLocal: true})
}

func titles(items []Item) []string {
	var out []string
	for _, it := range items {
		out = append(out, it.Title)
	}
	return out
}

// TestSearch_ShowNeverGoesOutWithoutTitleOrID checks every show search a
// Source gets has a title or an id it lists, and that a site ignoring the id
// is searched again by title.
func TestSearch_ShowNeverGoesOutWithoutTitleOrID(t *testing.T) {
	season := Query{Title: "Cabin in the Woods", TV: true, Season: 1, Words: "S01", TVDB: 4242, Categories: []int{5000}}
	for _, c := range []struct {
		name      string
		params    string
		ignoreIDs bool
		want      int // requests
	}{
		{"site takes the id", "q,season,ep,tvdbid", false, 1},
		{"site lists no id", "q,season,ep", false, 1},
		{"site lists the id but ignores it", "q,season,ep,tvdbid", true, 2},
		{"site lists no params", "", true, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := &capsSite{tvParams: c.params, ignoreIDs: c.ignoreIDs}
			items, err := s.start(t).Search(context.Background(), season)
			if err != nil {
				t.Fatal(err)
			}
			if len(s.queries) != c.want {
				t.Fatalf("queries %v", s.queries)
			}
			for _, q := range s.queries {
				if q.Get("q") == "" && q.Get("tvdbid") == "" || q.Get("cat") != "5000" || q.Get("t") != "tvsearch" {
					t.Fatalf("bad query %v", q)
				}
			}
			found := false
			for _, n := range titles(items) {
				found = found || n == notitleHits[0]
				for _, bad := range notitleLatest {
					if n == bad {
						t.Fatalf("kept the site's newest releases: %v", titles(items))
					}
				}
			}
			if !found {
				t.Fatalf("missed the season: %v", titles(items))
			}
		})
	}
}

// TestSearch_EmptyTitleNoID refuses a search that would ask for nothing.
func TestSearch_EmptyTitleNoID(t *testing.T) {
	s := &capsSite{tvParams: "q,season,ep"}
	cl := s.start(t)
	for _, q := range []Query{
		{Title: "!!!", TV: true, Season: 1},
		{Title: "", TV: true, TVDB: 7}, // the site lists no tvdbid
		{Title: "  "},
		{Title: "...", TV: true, Season: 1, Words: "S01"}, // text-only Source below
	} {
		if _, err := cl.Search(context.Background(), q); err == nil {
			t.Errorf("%+v: searched with nothing to search by", q)
		}
	}
	text := (&capsSite{}).startTextOnly(t)
	if _, err := text.Search(context.Background(), Query{Title: "...", TV: true, Season: 1, Words: "S01"}); err == nil {
		t.Error("searched for S01 alone")
	}
	if len(s.queries) != 0 {
		t.Fatalf("sent %v", s.queries)
	}
}

// startTextOnly is a Source with only a text search; any search fails the test.
func (s *capsSite) startTextOnly(t *testing.T) *Client {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("t") == "caps" {
			_, _ = w.Write([]byte(`<caps><searching><search available="yes" supportedParams="q"/></searching></caps>`))
			return
		}
		t.Errorf("sent %v", r.URL.Query())
		_, _ = w.Write([]byte(tvRSS()))
	})
	return New(Config{URL: srv.URL + "/api", Protocol: "torrent", AllowLocal: true})
}

// TestHandleSeason_SiteIgnoresID runs a "Choose a release" season search for
// a show that shares its name with a movie, on a site that answers an id-only
// search with its newest releases.
func TestHandleSeason_SiteIgnoresID(t *testing.T) {
	ts := &tvServer{answer: func(q url.Values) []string {
		if q.Get("q") == "" {
			return notitleLatest
		}
		return notitleHits
	}}
	f := seedShow(t, ts.start(t), nil)
	exec(t, f.fixture, "UPDATE titles SET name = 'Cabin in the Woods', sort_name = 'cabin in the woods'")
	runKind(t, f.fixture, &Searcher{Store: f.s}, jobs.KindSearchSeason, &domain.Subject{Type: "part", ID: f.parts[1]},
		map[string]any{"version_id": f.verID, "choose": false, "by_hand": true})
	for _, q := range ts.queries {
		if q.Get("q") == "" && q.Get("tvdbid") == "" || q.Get("cat") != "5000" {
			t.Fatalf("bad query %v", q)
		}
	}
	pack := relByName(t, f.fixture, notitleHits[0])
	if pack.State != "judged" || len(pack.MatchedSlotIDs) != 3 {
		t.Fatalf("season pack %s %v %v", pack.State, pack.MatchedSlotIDs, reasonCodes(pack))
	}
	var n int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM releases WHERE name IN (?, ?)", notitleLatest[0], notitleLatest[1]).Scan(&n)
	if n != 0 {
		t.Fatalf("kept %d of the site's newest releases", n)
	}
	if movie := relByName(t, f.fixture, notitleHits[1]); len(movie.MatchedSlotIDs) != 0 {
		t.Fatalf("movie release matched the show: %v", movie.MatchedSlotIDs)
	}
}

// TestMovieSearch_KindsAndCategories checks a movie search skips a Source set
// up only for shows, and uses the standard movie categories when the Source
// lists none.
func TestMovieSearch_KindsAndCategories(t *testing.T) {
	for _, c := range []struct {
		name, kinds, cats string
		want              []string // cat of each request
	}{
		{"no movie categories", `[]`, `{}`, []string{"2000"}},
		{"shows only", `["tv"]`, `{"tv":[5000]}`, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			var mu sync.Mutex
			var got []string
			srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				if q.Get("t") == "caps" {
					_, _ = w.Write([]byte(tvCaps))
					return
				}
				mu.Lock()
				got = append(got, q.Get("cat"))
				mu.Unlock()
				_, _ = w.Write([]byte(tvRSS()))
			})
			f := seed(t, srv.URL+"/api")
			exec(t, f, "UPDATE sources SET kinds = ?, categories = ?", c.kinds, c.cats)
			runKind(t, f, &Searcher{Store: f.s}, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slotID}, map[string]any{"by_hand": true})
			if len(got) != len(c.want) || len(got) > 0 && got[0] != c.want[0] {
				t.Fatalf("cats %v, want %v", got, c.want)
			}
		})
	}
}

// TestSearch_NothingToSearchKeepsSource checks a title with no usable name
// and no id is not sent, and does not pause the Source.
func TestSearch_NothingToSearchKeepsSource(t *testing.T) {
	sent := 0
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("t") == "caps" {
			_, _ = w.Write([]byte(tvCaps))
			return
		}
		sent++
		_, _ = w.Write([]byte(tvRSS()))
	})
	f := seed(t, srv.URL+"/api")
	exec(t, f, "UPDATE titles SET name = '!!!', source_ids = '{}'")
	runKind(t, f, &Searcher{Store: f.s}, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slotID}, map[string]any{"by_hand": true})
	var state string
	_ = f.s.Read().QueryRow("SELECT state FROM sources WHERE id = ?", f.srcID).Scan(&state)
	if sent != 0 || state != "enabled" {
		t.Fatalf("sent %d, Source %s", sent, state)
	}
}
