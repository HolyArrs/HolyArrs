package musictrend

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func mbidN(n int) string { return fmt.Sprintf("00000000-0000-0000-0000-%012d", n) }

func TestWarmerSeedsOrderAndCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base := map[string]int{"month": 100, "year": 200, "all_time": 300}[r.URL.Query().Get("range")]
		if r.URL.Query().Get("count") != "1000" {
			t.Errorf("count = %q", r.URL.Query().Get("count"))
		}
		fmt.Fprint(w, `{"payload":{"artists":[`)
		for i := range 5 {
			if i > 0 {
				fmt.Fprint(w, ",")
			}
			fmt.Fprintf(w, `{"artist_mbid":%q,"artist_name":"a"}`, mbidN(base+i))
		}
		fmt.Fprint(w, `]}}`)
	}))
	defer srv.Close()
	w := &Warmer{ListenBrainzURL: srv.URL, Max: 10, Log: func(string, ...any) {},
		Charts: func() []string {
			return []string{mbidN(1), mbidN(1), "bad", mbidN(2), mbidN(3), mbidN(4), mbidN(5), mbidN(6)}
		}}
	got := w.Seeds(context.Background())
	want := []string{mbidN(1), mbidN(2), mbidN(3), mbidN(4), mbidN(5), mbidN(100), mbidN(101), mbidN(102), mbidN(103), mbidN(104)}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("seeds = %v\nwant %v", got, want)
	}
}

func TestWarmerPassPacesOnlyFetches(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"payload":{"artists":[{"artist_mbid":%q,"artist_name":"a"},{"artist_mbid":%q,"artist_name":"b"},{"artist_mbid":%q,"artist_name":"c"}]}}`, mbidN(1), mbidN(2), mbidN(3))
	}))
	defer srv.Close()
	var calls []string
	w := &Warmer{ListenBrainzURL: srv.URL, Pace: 30 * time.Millisecond, Log: func(string, ...any) {},
		Warm: func(_ context.Context, id string) (bool, error) {
			calls = append(calls, id)
			switch id {
			case mbidN(1):
				return false, nil // kept fresh: no wait
			case mbidN(2):
				return true, nil
			}
			return true, errors.New("upstream_budget")
		}}
	began := time.Now()
	n, fetched, failed := w.Pass(context.Background())
	if n != 3 || fetched != 2 || failed != 1 || len(calls) != 3 {
		t.Fatalf("pass = %d %d %d calls %v", n, fetched, failed, calls)
	}
	// One paced fetch (30 ms) plus one failure (4 x 30 ms).
	if d := time.Since(began); d < 150*time.Millisecond {
		t.Fatalf("pass took %s, want the pacing", d)
	}
}

func TestChartArtists(t *testing.T) {
	s := &Service{}
	m := map[listKey]list{
		{"artist", "global"}: {body: []byte(fmt.Sprintf(`{"items":[{"mbid":%q}]}`, mbidN(7)))},
		{"album", "global"}:  {body: []byte(fmt.Sprintf(`{"items":[{"mbid":%q,"artist_mbid":%q}]}`, mbidN(8), mbidN(9)))},
	}
	s.lists.Store(&m)
	got := s.ChartArtists()
	if fmt.Sprint(got) != fmt.Sprint([]string{mbidN(7), mbidN(9)}) {
		t.Fatalf("chart artists = %v", got)
	}
}
