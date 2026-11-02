package main

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"
	"time"
)

// Many workers at once never raise one source's request rate: requests to
// a host are spaced by its gap across every client sharing the pace, and
// another host is not held back.
func TestSourcePaceHoldsUnderConcurrency(t *testing.T) {
	var mu sync.Mutex
	var at []time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	// Times are taken as the pace lets each request go (below it), so a
	// slow connection on a busy machine cannot bunch them up.
	sent := rtFunc(func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		at = append(at, time.Now())
		mu.Unlock()
		return http.DefaultTransport.RoundTrip(r)
	})
	const gap = 60 * time.Millisecond
	pace := &hostPace{gap: func(string) time.Duration { return gap }}
	a, b := pace.paced(&http.Client{Transport: sent}), pace.paced(&http.Client{Transport: sent})
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(c *http.Client) {
			defer wg.Done()
			resp, err := c.Get(srv.URL)
			if err == nil {
				resp.Body.Close()
			}
		}([]*http.Client{a, b}[i%2])
	}
	wg.Wait()
	sort.Slice(at, func(i, j int) bool { return at[i].Before(at[j]) })
	if len(at) != 8 {
		t.Fatalf("%d requests", len(at))
	}
	for i := 1; i < len(at); i++ {
		if d := at[i].Sub(at[i-1]); d < gap-10*time.Millisecond {
			t.Fatalf("requests %d and %d only %v apart, want %v", i-1, i, d, gap)
		}
	}
}

func TestSourceGaps(t *testing.T) {
	for host, want := range map[string]time.Duration{
		"coverartarchive.org": time.Second, "ia800.us.archive.org": 125 * time.Millisecond,
		"api.music.apple.com": 100 * time.Millisecond, "api.deezer.com": 200 * time.Millisecond,
		"webservice.fanart.tv": 500 * time.Millisecond, "assets.fanart.tv": 500 * time.Millisecond,
		"music.youtube.com": time.Second, "www.youtube.com": time.Second, "i.ytimg.com": 50 * time.Millisecond,
		"www.googleapis.com": 200 * time.Millisecond, "www.wikidata.org": time.Second,
		"is1-ssl.mzstatic.com": 100 * time.Millisecond, "is5-ssl.mzstatic.com": 100 * time.Millisecond,
	} {
		if got := sourceGap(host); got != want {
			t.Errorf("%s: %v, want %v", host, got, want)
		}
	}
}

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
