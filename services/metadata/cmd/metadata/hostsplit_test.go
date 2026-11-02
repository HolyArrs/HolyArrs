package main

import (
	"net/http"
	"strings"
	"testing"

	"github.com/CoderLuii/holyarrs-metadata/internal/budget"
)

type tagRT struct {
	tag string
	got *[]string
}

func (t tagRT) RoundTrip(r *http.Request) (*http.Response, error) {
	*t.got = append(*t.got, t.tag+":"+r.URL.Host)
	// The archive's asks redirect to its file hosts, as the real one does.
	if r.URL.Host == "coverartarchive.org" && strings.HasPrefix(r.URL.Path, "/redirect") {
		h := http.Header{}
		h.Set("Location", "https://ia800.us.archive.org/f.jpg")
		return &http.Response{StatusCode: 307, Header: h, Body: http.NoBody, Request: r}, nil
	}
	return &http.Response{StatusCode: 200, Body: http.NoBody, Request: r}, nil
}

// The Cover Art Archive and the archive.org file hosts it redirects to go
// over the 7-IP pool, each call metered; Apple, YouTube and Google hosts
// (and every other) stay on the paced client from the VM's own address.
func TestCoverArtArchiveOverPool(t *testing.T) {
	var got []string
	pool := &http.Client{Transport: tagRT{"pool", &got}}
	own := &http.Client{Transport: tagRT{"own", &got}}
	metered := 0
	c := poolForCAA(own, pool, func(*http.Request) error { metered++; return nil })
	for _, u := range []string{"https://coverartarchive.org/release-group/x/front-500", "https://ia800.us.archive.org/f.jpg",
		"https://archive.org/download/x", "https://api.music.apple.com/v1/x", "https://is1-ssl.mzstatic.com/a.jpg",
		"https://i.ytimg.com/vi/x/maxresdefault.jpg", "https://www.googleapis.com/youtube/v3/videos", "https://music.youtube.com/youtubei/v1/search",
		"https://notarchive.org/x"} {
		r, _ := http.NewRequest("GET", u, nil)
		if _, err := c.Do(r); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"pool:coverartarchive.org", "pool:ia800.us.archive.org", "pool:archive.org", "own:api.music.apple.com", "own:is1-ssl.mzstatic.com",
		"own:i.ytimg.com", "own:www.googleapis.com", "own:music.youtube.com", "own:notarchive.org"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("%v", got)
	}
	if metered != 3 {
		t.Fatalf("pool calls metered %d, want 3", metered)
	}
	if poolForCAA(own, nil, nil) != own {
		t.Fatal("no pool: the own client stays")
	}
}

// M7: a Cover Art Archive redirect to an archive.org file host follows
// over the pool too, never the VM's own address.
func TestCoverArtArchiveRedirectStaysOnPool(t *testing.T) {
	var got []string
	c := poolForCAA(&http.Client{Transport: tagRT{"own", &got}}, &http.Client{Transport: tagRT{"pool", &got}}, nil)
	r, _ := http.NewRequest("GET", "https://coverartarchive.org/redirect/front", nil)
	if _, err := c.Do(r); err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, " ") != "pool:coverartarchive.org pool:ia800.us.archive.org" {
		t.Fatalf("%v", got)
	}
}

// M8: pool calls are counted in the host pace numbers and stop when the
// upstream budget is spent.
func TestPoolMeterCountsAndBudgets(t *testing.T) {
	pace := &hostPace{gap: sourceGap}
	m := poolMeter(pace, nil)
	r, _ := http.NewRequest("GET", "https://coverartarchive.org/x", nil)
	if err := m(r); err != nil {
		t.Fatal(err)
	}
	pace.mu.Lock()
	n := pace.calls["coverartarchive.org"]
	pace.mu.Unlock()
	if n != 1 {
		t.Fatalf("calls %d", n)
	}
	b := budget.New(map[string]int{"deezer": 1})
	for i := 0; i < 1000 && b.Take("deezer"); i++ {
	}
	d, _ := http.NewRequest("GET", "https://api.deezer.com/x", nil)
	if err := poolMeter(pace, b)(d); err != budget.ErrSpent {
		t.Fatalf("spent budget: %v", err)
	}
}
