package sources

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A big Library is checked for shared names quickly: each of its names is
// cleaned once, and a name is then looked up, not compared with every show.
func TestSharedNamesOfABigLibraryAreQuick(t *testing.T) {
	var shows []mediafiles.ShowName
	for i := 0; i < 10000; i++ {
		for j := 0; j < 16; j++ {
			shows = append(shows, mediafiles.ShowName{ID: int64(i + 10), Name: fmt.Sprintf("Show Number %d: The Other Name %d", i, j), Year: 1990 + i%30})
		}
	}
	shows = append(shows, mediafiles.ShowName{ID: 1, Name: "The Harbour!", Year: 2021}, mediafiles.ShowName{ID: 2, Name: "S.W.A.T.", Year: 2015},
		mediafiles.ShowName{ID: 3, Name: "Copper Valley", Year: 2015}, mediafiles.ShowName{ID: 3, Name: "Own Other Name", Year: 2015})
	year := 2015
	// The process-wide kept name forms (releasename.NameForms) fill on the
	// first build; build once first so only the set itself is measured.
	newSharedNames(shows)
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	start := time.Now()
	lib := newSharedNames(shows)
	built := time.Since(start)
	runtime.GC()
	runtime.ReadMemStats(&after)
	var names []string
	for i := 0; i < 31; i++ {
		names = append(names, fmt.Sprintf("Some Other Name %d", i))
	}
	names = append(names, "A Harbour", "SWAT", "Show Number 77: The Other Name 3", "Own Other Name")
	got := map[string]string{}
	for _, n := range names {
		if shared, same := lib.has(n, 3, &year); shared {
			got[n] = fmt.Sprint(same)
		}
	}
	if fmt.Sprint(got) != "map[A Harbour:false SWAT:true Show Number 77: The Other Name 3:false]" {
		t.Fatalf("got %v", got)
	}
	// One cleaning per Library name; a lookup is then a map read.
	if lib.cleaned != len(shows) {
		t.Fatalf("%d names cleaned for %d Library names", lib.cleaned, len(shows))
	}
	held := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	if held > 12<<20 {
		t.Fatalf("the set holds %d bytes", held)
	}
	t.Logf("%d Library names: built in %s, holds %.1f MB", len(shows), built, float64(held)/(1<<20))
	runtime.KeepAlive(lib)
	runtime.KeepAlive(shows)
}

// A run of searches reads the Library's show names once: they are kept for a
// short time.
func TestLibraryNamesAreKeptBetweenSearches(t *testing.T) {
	f := seedShow(t, "http://127.0.0.1:1/api", nil)
	s := &Searcher{Store: f.s}
	first := s.libraryNames(context.Background())
	if again := s.libraryNames(context.Background()); again != first {
		t.Fatal("the names were read again straight away")
	}
	s.namesAt = time.Now().Add(-2 * sharedNamesFor)
	if again := s.libraryNames(context.Background()); again == first {
		t.Fatal("old names were not read again")
	}
	if shared, _ := first.has("Copper Valley", 0, nil); !shared {
		t.Fatal("the seeded show's name is missing")
	}
	if shared, _ := first.has("Copper Valley", f.titleID, nil); shared {
		t.Fatal("a show shares its name with itself")
	}
}

// The kept names are let go once they are old, so a big Library's names do
// not stay in memory between runs of searches.
func TestLibraryNamesAreLetGoWhenOld(t *testing.T) {
	f := seedShow(t, "http://127.0.0.1:1/api", nil)
	s := &Searcher{Store: f.s, namesFor: 20 * time.Millisecond}
	if s.libraryNames(context.Background()) == nil {
		t.Fatal("no names")
	}
	for end := time.Now().Add(5 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		s.namesMu.Lock()
		gone := s.names == nil
		s.namesMu.Unlock()
		if gone {
			return
		}
		if time.Now().After(end) {
			t.Fatal("the names are still kept")
		}
	}
}

// A read that fails (a search that was stopped) gives no shared names and is
// not kept: the next search reads the Library again.
func TestLibraryNamesFailedReadIsNotKept(t *testing.T) {
	f := seedShow(t, "http://127.0.0.1:1/api", nil)
	s := &Searcher{Store: f.s}
	stopped, stop := context.WithCancel(context.Background())
	stop()
	if lib := s.libraryNames(stopped); lib == nil || s.names != nil {
		t.Fatalf("a failed read: got %v, kept %v", lib, s.names)
	}
	if shared, _ := s.libraryNames(context.Background()).has("Copper Valley", 0, nil); !shared {
		t.Fatal("the next read did not see the Library")
	}
}

// A movie search asks by the movie's name and year first, and by its other
// name with the year only when that finds nothing.
func TestNameSearches_MovieAsksOtherNameWithYear(t *testing.T) {
	var mu sync.Mutex
	var asks []url.Values
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("t") == "caps" {
			_, _ = w.Write([]byte(`<caps><searching><search available="yes" supportedParams="q"/><movie-search available="yes" supportedParams="q"/></searching><categories><category id="2000" name="Movies"/></categories></caps>`))
			return
		}
		mu.Lock()
		asks = append(asks, q)
		mu.Unlock()
		if q.Get("q") != "Dune Part Two 2024" {
			_, _ = w.Write([]byte(tvRSS("Sand.Planet.2025.1080p.WEB-DL.H.264-QUILL")))
			return
		}
		_, _ = w.Write([]byte(tvRSS()))
	})
	f := seed(t, srv.URL+"/api")
	exec(t, f, `UPDATE titles SET original_language = 'fr', aliases = '["Sand Planet"]'`)
	for _, params := range []map[string]any{nil, {"by_hand": true, "choose": false}} {
		mu.Lock()
		asks = nil
		mu.Unlock()
		j := runKind(t, f, &Searcher{Store: f.s}, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slotID}, params)
		if j.State != "done" {
			t.Fatalf("job %s %+v", j.State, j.Error)
		}
		var st string
		if err := f.s.Read().QueryRow("SELECT state FROM slots WHERE id = ?", f.slotID).Scan(&st); err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		if len(asks) != 2 || asks[0].Get("q") != "Dune Part Two 2024" || asks[1].Get("q") != "sand planet 2024" {
			t.Fatalf("%v: want the movie's name, then its other name with the year, got %s after %v", params, st, asks)
		}
		mu.Unlock()
	}
}
