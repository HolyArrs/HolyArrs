package sources

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// countedSearches runs one episode search of the seeded show against a Source
// that finds nothing and returns the searches the Source was asked. The show
// has other names and the given original language.
func countedSearches(t *testing.T, caps, language string, params map[string]any) []url.Values {
	t.Helper()
	return countedRun(t, caps, language, func(t *testing.T, f showFix) domain.Job {
		return runKind(t, f.fixture, &Searcher{Store: f.s}, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slots["S01E02"]}, params)
	})
}

// countedRun is countedSearches for any search of the seeded show.
func countedRun(t *testing.T, caps, language string, run func(*testing.T, showFix) domain.Job) []url.Values {
	t.Helper()
	var mu sync.Mutex
	var asks []url.Values
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("t") == "caps" {
			_, _ = w.Write([]byte(caps))
			return
		}
		mu.Lock()
		asks = append(asks, q)
		mu.Unlock()
		_, _ = w.Write([]byte(tvRSS()))
	})
	f := seedShow(t, srv.URL+"/api", nil)
	err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE titles SET original_language = ?, aliases = '["Name One","Name Two","Name Three","Name Four"]' WHERE id = ?`, language, f.titleID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if j := run(t, f); j.State != "done" {
		t.Fatalf("job %s %+v", j.State, j.Error)
	}
	mu.Lock()
	defer mu.Unlock()
	return asks
}

const wordCaps = `<caps><searching><search available="yes" supportedParams="q"/><tv-search available="yes" supportedParams="q,season,ep"/></searching><categories><category id="5000" name="TV"/></categories></caps>`

// A Source asked by an id that finds nothing is asked by the title
// next, the id search first.
func TestNameSearches_IDSourceThatFindsNothingFallsBackToTheTitle(t *testing.T) {
	for _, params := range []map[string]any{nil, {"by_hand": true}, {"by_hand": true, "choose": false}} {
		asks := countedSearches(t, tvCaps, "ja", params)
		if len(asks) < 2 || asks[0].Get("tvdbid") != "4242" || asks[1].Get("q") != "Copper Valley" || asks[1].Get("tvdbid") != "" {
			t.Fatalf("%v: want the id search, then the title, got %d: %v", params, len(asks), asks)
		}
	}
}

// A Source that searches by words is asked one other name on an automatic
// search and more on a hand search.
func TestNameSearches_WordSource(t *testing.T) {
	for _, c := range []struct {
		params map[string]any
		want   int
	}{
		{nil, 1 + maxAutoNameSearches},
		{map[string]any{"by_hand": true}, 1 + maxNameSearches},
		{map[string]any{"by_hand": true, "choose": false}, 1 + maxNameSearches},
	} {
		asks := countedSearches(t, wordCaps, "ja", c.params)
		if len(asks) != c.want {
			t.Fatalf("%v: want %d searches, got %d: %v", c.params, c.want, len(asks), asks)
		}
		if asks[1].Get("q") != "name one" || asks[1].Get("season") != "1" || asks[1].Get("ep") != "2" {
			t.Fatalf("the first other name asked is wrong: %v", asks[1])
		}
	}
}

// An English title's other names are its names in other countries, where the
// releases are dubbed: they are not searched.
func TestNameSearches_EnglishTitleAsksNoOtherNames(t *testing.T) {
	asks := countedSearches(t, wordCaps, "en", map[string]any{"by_hand": true, "choose": false})
	if len(asks) != 1 {
		t.Fatalf("want the title search only, got %d: %v", len(asks), asks)
	}
}

// A title in a language written in Latin letters has one own-language name,
// first in its list; the names after it are its names in other countries, so
// a hand search asks that one only. A language written in other letters has
// several romanised spellings, and a hand search asks more of them.
func TestNameSearches_LatinLanguageAsksOneOtherName(t *testing.T) {
	hand := map[string]any{"by_hand": true, "choose": false}
	asks := countedSearches(t, wordCaps, "es", hand)
	if len(asks) != 2 || asks[1].Get("q") != "name one" {
		t.Fatalf("want the title and its first other name, got %d: %v", len(asks), asks)
	}
	if asks = countedSearches(t, wordCaps, "ko", hand); len(asks) != 1+maxNameSearches {
		t.Fatalf("want %d searches, got %d: %v", 1+maxNameSearches, len(asks), asks)
	}
}

// A season search that also asks another season release names use asks the
// other names once, not again for that season.
func TestNameSearches_SeasonAsksOtherNamesOnce(t *testing.T) {
	asks := countedRun(t, wordCaps, "ja", func(t *testing.T, f showFix) domain.Job {
		setAlt(t, f, "S01E03", 3, 1)
		return runKind(t, f.fixture, &Searcher{Store: f.s}, jobs.KindSearchSeason, &domain.Subject{Type: "part", ID: f.parts[1]}, map[string]any{"version_id": f.verID, "by_hand": true, "choose": false})
	})
	seasons := map[string]int{}
	for _, a := range asks {
		seasons[a.Get("season")]++
	}
	if seasons["1"] != 1+maxNameSearches || seasons["3"] != 1 || len(asks) != 2+maxNameSearches {
		t.Fatalf("want %d searches of season 1 and 1 of season 3, got %v: %v", 1+maxNameSearches, seasons, asks)
	}
}

// A Source that answers the title search and then asks to slow down on an
// other name keeps what it found and is paused, and its failures do not add
// up from one search to the next: the title search worked.
func TestNameSearches_SlowedOtherNameDoesNotAddUp(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch {
		case q.Get("t") == "caps":
			_, _ = w.Write([]byte(wordCaps))
		case q.Get("q") == "Copper Valley":
			_, _ = w.Write([]byte(tvRSS(fmt.Sprintf("Copper.Valley.S01E%02s.1080p.WEB-DL.H.264-QUILL", q.Get("ep")))))
		default:
			w.WriteHeader(http.StatusTooManyRequests)
		}
	})
	f := seedShow(t, srv.URL+"/api", nil)
	exec(t, f.fixture, `UPDATE titles SET original_language = 'ja', aliases = '["Name One","Name Two"]' WHERE id = ?`, f.titleID)
	for _, label := range []string{"S01E02", "S01E03"} {
		exec(t, f.fixture, `UPDATE sources SET paused_until = '2000-01-01T00:00:00Z' WHERE state = 'paused'`)
		j := runKind(t, f.fixture, &Searcher{Store: f.s}, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slots[label]}, nil)
		if j.State != "done" {
			t.Fatalf("%s: job %s %+v", label, j.State, j.Error)
		}
		relByName(t, f.fixture, "Copper.Valley."+label+".1080p.WEB-DL.H.264-QUILL")
		var state string
		var failures sql.NullString
		if err := f.s.Read().QueryRow(`SELECT state, json_extract(extra, '$.failures') FROM sources WHERE id = ?`, f.srcID).Scan(&state, &failures); err != nil {
			t.Fatal(err)
		}
		if state != "paused" || failures.String != "1" {
			t.Fatalf("%s: want the Source paused with 1 failure, got %s with %q", label, state, failures.String)
		}
	}
}
