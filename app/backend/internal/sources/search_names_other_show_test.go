package sources

import (
	"context"
	"database/sql"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// addShow adds one more show to the Library, with a year and other names.
func addShow(t *testing.T, f showFix, name string, year int, aliases ...string) {
	t.Helper()
	if aliases == nil {
		aliases = []string{}
	}
	ti := &mediafiles.Title{Kind: "show", GateDriver: "none", Name: name, SortName: name, Year: ptr(year),
		Aliases: aliases, SourceIDs: map[string]string{}, MetadataSource: "tvmaze", NewPartsPolicy: "all", ReleaseGate: "released",
		MusicTypes: []string{}, WhyAdded: "by_hand", Artwork: []string{}, Status: "running", State: "active"}
	if err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := database.Insert(tx, "titles", ti, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

// otherNameSearch runs an automatic search of S01E02 of the seeded show
// (Copper Valley, 2015) with the language and one other name, against a
// word-only Source that holds release under the words held. It returns the
// searches asked and the Slot's state after.
func otherNameSearch(t *testing.T, language, otherName, held, release string, prep func(showFix)) ([]url.Values, mediafiles.SlotState) {
	t.Helper()
	var mu sync.Mutex
	var asks []url.Values
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("t") == "caps" {
			_, _ = w.Write([]byte(wordCaps))
			return
		}
		mu.Lock()
		asks = append(asks, q)
		mu.Unlock()
		if q.Get("q") == held {
			_, _ = w.Write([]byte(tvRSS(release)))
			return
		}
		_, _ = w.Write([]byte(tvRSS()))
	})
	f := seedShow(t, srv.URL+"/api", nil)
	exec(t, f.fixture, `UPDATE titles SET original_language = ?, aliases = json_array(?) WHERE id = ?`, language, otherName, f.titleID)
	if prep != nil {
		prep(f)
	}
	j := runKind(t, f.fixture, &Searcher{Store: f.s}, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slots["S01E02"]}, nil)
	if j.State != "done" {
		t.Fatalf("job %s %+v", j.State, j.Error)
	}
	mu.Lock()
	defer mu.Unlock()
	return asks, f.state(t, "S01E02")
}

// A release found under an other name that carries a year is handed to the
// judge like any other: the judge decides whether the year fits the show, the
// search does not. Releases of the show named with a later year arrive too.
func TestNameSearches_OtherNameReleaseWithAYearGoesToTheJudge(t *testing.T) {
	for _, release := range []string{
		"The.Harbour.2021.S01E02.1080p.WEB-DL.H.264-QUILL",
		"The.Harbour.2018.S01E02.1080p.WEB-DL.H.264-QUILL",
	} {
		var f showFix
		asks, _ := otherNameSearch(t, "da", "The Harbour", "harbour", release, func(sf showFix) { f = sf })
		if len(asks) != 2 || asks[0].Get("q") != "Copper Valley" || asks[1].Get("q") != "harbour" {
			t.Fatalf("want the title and the other name asked without a year, got %v", asks)
		}
		relByName(t, f.fixture, release)
	}
}

// A release with no year found under an other name is the show's, as long as
// no other show in the Library carries that name.
func TestNameSearches_OtherNameReleaseWithoutAYearIsTaken(t *testing.T) {
	if asks, st := otherNameSearch(t, "da", "The Harbour", "harbour", "The.Harbour.S01E02.1080p.WEB-DL.H.264-QUILL", nil); st != mediafiles.SlotChosen {
		t.Fatalf("slot %s after %v", st, asks)
	}
}

// An other name that another Library show of the same year carries, as its
// name or as an other name, is not asked: nothing could tell the two apart.
func TestNameSearches_NameOfASameYearLibraryShowIsNotAsked(t *testing.T) {
	for _, asAlias := range []bool{false, true} {
		asks, st := otherNameSearch(t, "da", "The Harbour", "harbour", "The.Harbour.S01E02.1080p.WEB-DL.H.264-QUILL", func(f showFix) {
			if asAlias {
				addShow(t, f, "Havnen", 2015, "The Harbour")
			} else {
				addShow(t, f, "The Harbour", 2015)
			}
		})
		if len(asks) != 1 || st == mediafiles.SlotChosen {
			t.Fatalf("as other name %v: want the title search only and nothing chosen, got %s after %v", asAlias, st, asks)
		}
	}
}

// When the other Library show is from another year the name is asked, since a
// year in a release name tells the two apart; a release with no year is then
// nobody's, and is not chosen.
func TestNameSearches_NameOfAnotherYearLibraryShowIsAskedNotGuessed(t *testing.T) {
	asks, st := otherNameSearch(t, "da", "The Harbour", "harbour", "The.Harbour.S01E02.1080p.WEB-DL.H.264-QUILL", func(f showFix) {
		addShow(t, f, "The Harbour", 2021)
	})
	if len(asks) != 2 {
		t.Fatalf("want the other name asked, got %v", asks)
	}
	if st == mediafiles.SlotChosen {
		t.Fatal("a release that two Library shows could own was chosen")
	}
	var f showFix
	const dated = "The.Harbour.2015.S01E02.1080p.WEB-DL.H.264-QUILL"
	otherNameSearch(t, "da", "The Harbour", "harbour", dated, func(sf showFix) {
		f = sf
		addShow(t, f, "The Harbour", 2021)
	})
	relByName(t, f.fixture, dated)
}

// A show is still found under its romanised or own-language name, with no
// year in the release name or with the show's own year.
func TestNameSearches_OwnOtherNameStillFound(t *testing.T) {
	for _, c := range []struct{ language, name, held, release string }{
		{"ja", "Doukoku no Tani", "doukoku no tani", "Doukoku.no.Tani.S01E02.1080p.WEB-DL.H.264-QUILL"},
		{"ja", "Doukoku no Tani", "doukoku no tani", "Doukoku.no.Tani.2015.S01E02.1080p.WEB-DL.H.264-QUILL"},
		{"es", "La casa de papel", "la casa de papel", "La.Casa.de.Papel.S01E02.1080p.WEB-DL.H.264-QUILL"},
	} {
		asks, st := otherNameSearch(t, c.language, c.name, c.held, c.release, nil)
		if st != mediafiles.SlotChosen {
			t.Fatalf("%s: slot %s after %v", c.release, st, asks)
		}
	}
}

// Releases of the show named with a later year, or with the year in brackets,
// reach the judge: the search drops none of them.
func TestNameSearches_OwnReleasesWithAnyYearReachTheJudge(t *testing.T) {
	for _, release := range []string{"Doukoku.no.Tani.2018.S01E02.1080p.WEB-DL.H.264-QUILL", "[EMBER] Doukoku no Tani (2017) - S01E02 [1080p]"} {
		var f showFix
		otherNameSearch(t, "ja", "Doukoku no Tani", "doukoku no tani", release, func(sf showFix) { f = sf })
		relByName(t, f.fixture, release)
	}
}

// Other names are for shows: a movie asks none and needs no Library check.
func TestNameAskIsForShowsOnly(t *testing.T) {
	f := seedShow(t, "http://127.0.0.1:1/api", nil)
	year := 2015
	a := (&Searcher{Store: f.s}).nameAsk(context.Background(), mediafiles.Title{Kind: "movie", Year: &year, OriginalLanguage: ptr("da")}, true)
	if a.n != 0 || a.shared != nil {
		t.Fatalf("a movie: %+v", a)
	}
	if a = (&Searcher{Store: f.s}).nameAsk(context.Background(), mediafiles.Title{Kind: "show", Year: &year, OriginalLanguage: ptr("da")}, false); a.n != 1 || a.shared == nil {
		t.Fatalf("a show: %+v", a)
	}
}

// The Library is read for other shows' names only when a Source is about to
// be asked by words, not for a Source asked by an id.
func TestOtherNamesLibraryCheckIsLazy(t *testing.T) {
	t.Parallel()
	for caps, want := range map[string]int{aliasCapsIDs: 0, wordCaps: 1} {
		// The site knows the show by its id, so an id search finds it and no
		// search by title follows (one that finds nothing falls back to words).
		srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("t") == "caps" {
				_, _ = w.Write([]byte(caps))
				return
			}
			_, _ = w.Write([]byte(`<rss><channel><item><title>Copper.Valley.S01.1080p.WEB-DL</title><guid>g1</guid><link>magnet:?xt=urn:btih:` + strings.Repeat("1", 40) + `</link></item></channel></rss>`))
		})
		c := New(Config{URL: srv.URL, Protocol: "torrent", AllowLocal: true})
		calls := 0
		q := Query{Title: "Copper Valley", TV: true, Season: 1, TVDB: 4242, Aliases: []string{"The Harbour"}}
		q.other.shared = func(context.Context, string) (bool, bool) { calls++; return false, false }
		if _, _, err := c.searchNamed(context.Background(), q, 1); err != nil {
			t.Fatal(err)
		}
		if calls != want {
			t.Fatalf("caps %q: %d Library checks, want %d", caps, calls, want)
		}
	}
}

// A very short other name finds anything; it is not asked unless it has two words.
func TestOtherNamesVeryShortOnesSkipped(t *testing.T) {
	t.Parallel()
	got := otherNameTexts("Some Show", []string{"DT", "D.T.", "Go On", "Abc"}, 5)
	if len(got) != 2 || got[0] != "go on" || got[1] != "abc" {
		t.Fatalf("got %q", got)
	}
}

// The year in a release found under an other name must be the show's own
// (2015): another year is refused, also one year off. With a show of that
// name and another year in the Library, a release with no year is nobody's.
func TestNameSearches_YearUnderAnOtherName(t *testing.T) {
	for _, c := range []struct {
		library int // the year of a Library show called The Harbour; 0: none
		release string
		taken   bool
	}{
		{0, "The.Harbour.2021.S01E02.1080p.WEB-DL.H.264-QUILL", false},
		{0, "The.Harbour.2016.S01E02.1080p.WEB-DL.H.264-QUILL", false},
		{0, "The.Harbour.2015.S01E02.1080p.WEB-DL.H.264-QUILL", true},
		{0, "The.Harbour.S01E02.1080p.WEB-DL.H.264-QUILL", true},
		{2021, "The.Harbour.S01E02.1080p.WEB-DL.H.264-QUILL", false},
		{2021, "The.Harbour.2015.S01E02.1080p.WEB-DL.H.264-QUILL", true},
		{2021, "The.Harbour.2021.S01E02.1080p.WEB-DL.H.264-QUILL", false},
		{0, "Doukoku.no.Tani.2016.S01E02.1080p.WEB-DL.H.264-QUILL", false},
	} {
		name, held := "The Harbour", "harbour"
		if c.release[0] == 'D' {
			name, held = "Doukoku no Tani", "doukoku no tani"
		}
		asks, st := otherNameSearch(t, "da", name, held, c.release, func(f showFix) {
			if c.library > 0 {
				addShow(t, f, "The Harbour", c.library)
			}
		})
		if len(asks) != 2 || (st == mediafiles.SlotChosen) != c.taken {
			t.Fatalf("Library show %d, %s: slot %s, want taken %v, after %v", c.library, c.release, st, c.taken, asks)
		}
	}
}

// Another Library show carries the name however it is spelled: the names are
// compared the way the judge compares them.
func TestNameSearches_SharedNameInAnySpelling(t *testing.T) {
	for _, c := range []struct {
		name  string
		alias []string
	}{{"Harbour", nil}, {"The Harbour!", nil}, {"the harbour", nil}, {"Havnen", []string{"A Harbour"}}} {
		asks, st := otherNameSearch(t, "da", "The Harbour", "harbour", "The.Harbour.S01E02.1080p.WEB-DL.H.264-QUILL", func(f showFix) {
			addShow(t, f, c.name, 2015, c.alias...)
		})
		if len(asks) != 1 || st == mediafiles.SlotChosen {
			t.Fatalf("same year, %q %v: want the name not asked, got %s after %v", c.name, c.alias, st, asks)
		}
		asks, st = otherNameSearch(t, "da", "The Harbour", "harbour", "The.Harbour.S01E02.1080p.WEB-DL.H.264-QUILL", func(f showFix) {
			addShow(t, f, c.name, 2021, c.alias...)
		})
		if len(asks) != 2 || st == mediafiles.SlotChosen {
			t.Fatalf("another year, %q %v: want the name asked and nothing chosen, got %s after %v", c.name, c.alias, st, asks)
		}
	}
}
