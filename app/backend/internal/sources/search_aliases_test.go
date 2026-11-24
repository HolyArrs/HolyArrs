package sources

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// aliasSite is a text-only Source: it answers a search with the releases that
// hold every word of q, and records each search it was asked.
type aliasSite struct {
	mu   sync.Mutex
	asks []url.Values
}

func (a *aliasSite) handler(releases []string, caps string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		v := r.URL.Query()
		if v.Get("t") == "caps" {
			_, _ = w.Write([]byte(caps))
			return
		}
		a.mu.Lock()
		a.asks = append(a.asks, v)
		a.mu.Unlock()
		var out strings.Builder
		for i, rel := range releases {
			ok := v.Get("tvdbid") == "" && v.Get("imdbid") == "" // the site does not know the title by id
			for _, word := range strings.Fields(strings.ToLower(v.Get("q"))) {
				ok = ok && strings.Contains(strings.ToLower(strings.ReplaceAll(rel, ".", " ")), word)
			}
			if ok {
				fmt.Fprintf(&out, `<item><title>%s</title><guid>g%d</guid><link>magnet:?xt=urn:btih:%040d</link></item>`, rel, i, i)
			}
		}
		_, _ = w.Write([]byte(`<rss><channel>` + out.String() + `</channel></rss>`))
	}
}

const aliasCapsText = `<caps><searching><search available="yes" supportedParams="q"/><tv-search available="yes" supportedParams="q,season,ep"/></searching></caps>`
const aliasCapsIDs = `<caps><searching><search available="yes" supportedParams="q"/><tv-search available="yes" supportedParams="q,season,ep,tvdbid"/></searching></caps>`

var icebladeAliases = []string{"冰剣の魔術師が世界を統べる", "Hyouken no Majutsushi ga Sekai wo Suberu", "Hyōken no Majutsushi ga Sekai o Suberu II"}

func icebladeQuery() Query {
	return Query{Title: "The Iceblade Sorcerer Shall Rule the World", TV: true, Season: 1, Episode: "2", TVDB: 411946, Aliases: icebladeAliases}
}

func searchWithOtherNames(t *testing.T, site *aliasSite, rels []string, caps string, q Query) []Item {
	t.Helper()
	srv := serve(t, site.handler(rels, caps))
	c := New(Config{URL: srv.URL, Protocol: "torrent", AllowLocal: true})
	items, slowed, err := c.searchNamed(context.Background(), q, maxNameSearches)
	if err != nil || slowed != nil {
		t.Fatal(err, slowed)
	}
	return items
}

// A Source that only searches by words finds a show released under its
// Japanese name when that name is one of the title's other names.
func TestOtherNamesSearched(t *testing.T) {
	t.Parallel()
	site := &aliasSite{}
	got := searchWithOtherNames(t, site, []string{"Hyouken.no.Majutsushi.ga.Sekai.wo.Suberu.S01E02.1080p.WEB.h264-GRP"}, aliasCapsText, icebladeQuery())
	if len(got) != 1 {
		t.Fatalf("want the release found under the other name, got %d", len(got))
	}
	if len(site.asks) < 2 {
		t.Fatalf("want the title search and searches by the other names, got %d: %v", len(site.asks), site.asks)
	}
	second := site.asks[1]
	if second.Get("q") != "hyouken no majutsushi ga sekai wo suberu" || second.Get("tvdbid") != "" || second.Get("season") != "1" || second.Get("ep") != "2" {
		t.Fatalf("the search by the other name is wrong: %v", second)
	}
}

// The other names are asked even when the title already finds a release: a
// site may hold one poor copy under the English name and the good ones under
// the Japanese name.
func TestOtherNamesSearchedWhenTitleFindsACopy(t *testing.T) {
	t.Parallel()
	site := &aliasSite{}
	got := searchWithOtherNames(t, site, []string{
		"The.Iceblade.Sorcerer.Shall.Rule.the.World.S01E02.480p.WEB.h264-GRP",
		"Hyouken.no.Majutsushi.ga.Sekai.wo.Suberu.S01E02.1080p.WEB.h264-GRP",
	}, aliasCapsText, icebladeQuery())
	if len(got) != 2 || len(site.asks) < 2 {
		t.Fatalf("want both releases and a search by the other name, got %d releases from %d searches", len(got), len(site.asks))
	}
}

// Other names are left out once the Source's window is mostly used, so they
// never take the searches the title needs.
func TestOtherNamesLeftOutWhenWindowIsFull(t *testing.T) {
	t.Parallel()
	site := &aliasSite{}
	srv := serve(t, site.handler(nil, aliasCapsText))
	c := New(Config{URL: srv.URL, Protocol: "torrent", AllowLocal: true, PerWindow: 10, Window: time.Hour})
	q := icebladeQuery()
	now := time.Now()
	for i := 0; i < 5; i++ {
		c.hits = append(c.hits, now)
	}
	if _, err := searchOtherNames(context.Background(), c, q, nil, maxNameSearches); err != nil {
		t.Fatal(err)
	}
	if len(site.asks) != 0 {
		t.Fatalf("want no search with the window half used, got %d", len(site.asks))
	}
}

// A Source that asks to slow down on an other-name search keeps what was found
// and the caller gets the error to pause it.
func TestOtherNamesSlowDownIsReturned(t *testing.T) {
	t.Parallel()
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("t") == "caps" {
			_, _ = w.Write([]byte(aliasCapsText))
			return
		}
		w.WriteHeader(http.StatusTooManyRequests)
	})
	c := New(Config{URL: srv.URL, Protocol: "torrent", AllowLocal: true})
	kept := []Item{{Title: "kept"}}
	got, err := searchOtherNames(context.Background(), c, icebladeQuery(), kept, maxNameSearches)
	var se *Error
	if !errors.As(err, &se) || se.Kind != KindRateLimited || len(got) != 1 {
		t.Fatalf("want the kept item and a rate_limited error, got %d items and %v", len(got), err)
	}
}

// Other names that no index could use are not sent: another script, a name
// that reads the same as one already tried, and anything past the limit.
func TestOtherNamesAreFewAndPlain(t *testing.T) {
	t.Parallel()
	site := &aliasSite{}
	q := icebladeQuery()
	q.Aliases = []string{"冰剣の魔術師が世界を統べる", "Волшебник ледяного клинка", "Name One", "name one", "Name Two", "Name Three", "Name Four", "Name Five"}
	got := searchWithOtherNames(t, site, nil, aliasCapsText, q)
	if len(got) != 0 {
		t.Fatalf("nothing to find, got %d", len(got))
	}
	if len(site.asks) != 1+maxNameSearches {
		t.Fatalf("want the title search and %d others, got %d: %v", maxNameSearches, len(site.asks), site.asks)
	}
	seen := map[string]bool{}
	for _, v := range site.asks[1:] {
		if seen[v.Get("q")] || strings.ContainsAny(v.Get("q"), "冰剣Волшебник") {
			t.Fatalf("unusable or repeated name sent: %v", site.asks)
		}
		seen[v.Get("q")] = true
	}
}

// A Source that fails on the other names does not take the title's answer away.
func TestOtherNamesFailureKeepsTheAnswer(t *testing.T) {
	t.Parallel()
	var n int
	var mu sync.Mutex
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("t") == "caps" {
			_, _ = w.Write([]byte(aliasCapsText))
			return
		}
		mu.Lock()
		n++
		first := n == 1
		mu.Unlock()
		if first {
			_, _ = w.Write([]byte(`<rss><channel></channel></rss>`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	})
	c := New(Config{URL: srv.URL, Protocol: "torrent", AllowLocal: true})
	q := icebladeQuery()
	items, err := c.Search(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := searchOtherNames(context.Background(), c, q, items, maxNameSearches); len(got) != 0 || n != 2 {
		t.Fatalf("want no items and one failed extra search, got %d items and %d searches", len(got), n)
	}
}

// The names to try are the plain ones, in the metadata's order: a longer name
// that holds a shorter one's words (a season's name) adds nothing, so the cap
// goes to names that differ ("wo" and "o").
func TestOtherNamesPickShortDistinctOnes(t *testing.T) {
	t.Parallel()
	title := "The Iceblade Sorcerer Shall Rule the World"
	aliases := []string{
		"Hyouken no Majutsushi ga Sekai wo Suberu", "冰剣の魔術師が世界を統べる", "Hyouken no Majutsushi ga Sekai wo Suberu II",
		"The Iceblade Sorcerer Shall Rule the World II", "Hyouken No Majutsushi Ga Sekai O Suberu",
		"Hyōken no Majutsushi ga Sekai wo Suberu - Sekai Saikyō no Majutsushi de Aru Shōnen wa", "Pháp Sư Băng Kiếm",
	}
	got := otherNameTexts(title, aliases, maxNameSearches)
	want := []string{"hyouken no majutsushi ga sekai wo suberu", "hyouken no majutsushi ga sekai o suberu", "hyoken no majutsushi ga sekai wo suberu sekai saikyo no majutsushi de aru shonen wa"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %q want %q", got, want)
	}
}

// A longer name left out for a shorter one later in the list gives that
// shorter name its place.
func TestOtherNamesShorterNameTakesThePlace(t *testing.T) {
	t.Parallel()
	got := otherNameTexts("Some Show", []string{"Romaji Name II", "German Name", "Romaji Name"}, 1)
	if len(got) != 1 || got[0] != "romaji name" {
		t.Fatalf("got %q", got)
	}
}

// One letter before an apostrophe stays a word of its own.
func TestPlainNameKeepsALetterBeforeAnApostrophe(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]string{
		"L'Auberge espagnole": "l auberge espagnole", "Le Fabuleux Destin d’Amélie": "le fabuleux destin d amelie",
		"Grey's Anatomy": "greys anatomy", "Don't Look": "dont look",
	} {
		if got := plainName(name); got != want {
			t.Fatalf("%s: got %q want %q", name, got, want)
		}
	}
}

// A movie whose own name finds nothing is asked by its other name with its
// year, as the older movie app does.
func TestOtherNamesAskedForAMovieFoundByNothingElse(t *testing.T) {
	t.Parallel()
	site := &aliasSite{}
	srv := serve(t, site.handler([]string{"L.Auberge.Espagnole.2002.1080p.BluRay.x264-GRP"}, `<caps><searching><search available="yes" supportedParams="q"/><movie-search available="yes" supportedParams="q"/></searching></caps>`))
	c := New(Config{URL: srv.URL, Protocol: "torrent", AllowLocal: true})
	q := Query{Title: "The Spanish Apartment", Year: 2002, IMDb: "tt0283900", Aliases: []string{"L'Auberge espagnole"}}
	items, _, err := c.searchNamed(context.Background(), q, maxNameSearches)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || len(site.asks) != 2 || site.asks[1].Get("q") != "l auberge espagnole 2002" {
		t.Fatalf("want the movie's name, then its other name with the year, got %d releases from %v", len(items), site.asks)
	}
}

// Odd names from outside are not sent: empty ones, marks only, and a very long one.
func TestOtherNamesOddOnesSkipped(t *testing.T) {
	t.Parallel()
	got := otherNameTexts("Some Show", []string{"", "   ", "!!!", "...", strings.Repeat("long name ", 500), "Some Show", "some  show!", "Real Other Name"}, maxNameSearches)
	if len(got) != 1 || got[0] != "real other name" {
		t.Fatalf("got %q", got)
	}
}

// A Source that answered by an id names the title however a release is named,
// so the other names are not asked.
func TestOtherNamesNotAskedAfterAnIDAnswer(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var asks []url.Values
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		v := r.URL.Query()
		if v.Get("t") == "caps" {
			_, _ = w.Write([]byte(aliasCapsIDs))
			return
		}
		mu.Lock()
		asks = append(asks, v)
		mu.Unlock()
		_, _ = w.Write([]byte(`<rss><channel><item><title>The.Iceblade.Sorcerer.Shall.Rule.the.World.S01E02.1080p.WEB.h264-GRP</title><guid>g</guid><link>magnet:?xt=urn:btih:0000000000000000000000000000000000000001</link></item></channel></rss>`))
	})
	c := New(Config{URL: srv.URL, Protocol: "torrent", AllowLocal: true})
	got, slowed, err := c.searchNamed(context.Background(), icebladeQuery(), maxNameSearches)
	if err != nil || slowed != nil || len(got) != 1 || len(asks) != 1 || asks[0].Get("tvdbid") == "" {
		t.Fatalf("want one id search and its release, got %d items from %d searches (%v, %v)", len(got), len(asks), err, slowed)
	}
}

// An automatic search asks about one other name, a hand search about more.
func TestOtherNamesAutomaticSearchAsksOne(t *testing.T) {
	t.Parallel()
	site := &aliasSite{}
	srv := serve(t, site.handler(nil, aliasCapsText))
	c := New(Config{URL: srv.URL, Protocol: "torrent", AllowLocal: true})
	q := icebladeQuery()
	q.Aliases = []string{"Name One", "Name Two", "Name Three"}
	if _, _, err := c.searchNamed(context.Background(), q, maxAutoNameSearches); err != nil {
		t.Fatal(err)
	}
	if len(site.asks) != 1+maxAutoNameSearches {
		t.Fatalf("want the title search and %d other name, got %d", maxAutoNameSearches, len(site.asks))
	}
}

// A Source that stops answering costs one extra wait, not one per name.
func TestOtherNamesStopAfterATimeout(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	n := 0
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("t") == "caps" {
			_, _ = w.Write([]byte(aliasCapsText))
			return
		}
		mu.Lock()
		n++
		first := n == 1
		mu.Unlock()
		if first {
			_, _ = w.Write([]byte(`<rss><channel></channel></rss>`))
			return
		}
		time.Sleep(600 * time.Millisecond)
	})
	c := New(Config{URL: srv.URL, Protocol: "torrent", AllowLocal: true, Timeout: 200 * time.Millisecond})
	q := icebladeQuery()
	q.Aliases = []string{"Name One", "Name Two", "Name Three"}
	if _, _, err := c.searchNamed(context.Background(), q, maxNameSearches); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if n != 2 {
		t.Fatalf("want the title search and one timed out search, got %d", n)
	}
}

func TestNameSearchesByTitle(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		language *string
		hand     bool
		want     int
	}{
		{ptr("en"), true, 0}, {ptr("en"), false, 0},
		{ptr("ja"), true, maxNameSearches}, {ptr("ja"), false, maxAutoNameSearches},
		{nil, true, maxNameSearches}, {ptr(""), false, maxAutoNameSearches},
		{ptr("es"), true, 1}, {ptr("es"), false, maxAutoNameSearches},
		{ptr("ko"), true, maxNameSearches}, {ptr("ru"), true, maxNameSearches},
	} {
		if got := nameSearches(mediafiles.Title{OriginalLanguage: c.language}, c.hand); got != c.want {
			t.Fatalf("language %v hand %v: got %d want %d", c.language, c.hand, got, c.want)
		}
	}
}
