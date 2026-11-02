package main

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// hostPace keeps every picture source to its own request rate, however
// many picture workers run at once: one pace per host, shared by every
// client given it, each request waiting for its host's next slot.
type hostPace struct {
	gap func(host string) time.Duration
	mu  sync.Mutex
	at  map[string]time.Time
	// waited and calls sum each host's pace waits (waitsLine).
	waited map[string]time.Duration
	calls  map[string]int
}

// waitsLine answers the hosts' pace waits since the last call, the most
// waited first ("host=calls/avg ms"), and starts again.
func (p *hostPace) waitsLine() string {
	p.mu.Lock()
	w, n := p.waited, p.calls
	p.waited, p.calls = nil, nil
	p.mu.Unlock()
	hosts := make([]string, 0, len(n))
	for h := range n {
		hosts = append(hosts, h)
	}
	sort.Slice(hosts, func(i, j int) bool { return w[hosts[i]] > w[hosts[j]] })
	parts := []string{}
	for i, h := range hosts {
		if i == 8 {
			break
		}
		parts = append(parts, fmt.Sprintf("%s=%d/%d", h, n[h], w[h].Milliseconds()/int64(n[h])))
	}
	return strings.Join(parts, " ")
}

// sourceGap is the least time between two requests to one picture source.
func sourceGap(host string) time.Duration {
	h := strings.ToLower(host)
	has := func(s string) bool { return h == s || strings.HasSuffix(h, "."+s) }
	switch {
	case has("coverartarchive.org"):
		// One a second: it shares MetaBrainz's gateway (one address) with
		// musicbrainz.org and ListenBrainz, which cut the VM off on
		// 2026-10-27 while pictures ran at 4 a second.
		return time.Second
	case has("archive.org"):
		return 125 * time.Millisecond // the file hosts the archive redirects to
	case has("mzstatic.com"):
		// Apple's picture hosts, at Apple's own 10 a second (the 250 ms
		// default made each download wait about 1 s once the fill ran).
		return 100 * time.Millisecond
	case has("api.music.apple.com"):
		return 100 * time.Millisecond // 10 a second (0 errors at 10/s on 2026-10-28; 429 backs it off)
	case has("api.deezer.com"):
		return 200 * time.Millisecond // half of 50 per 5 s
	case has("fanart.tv"):
		return 500 * time.Millisecond
	case has("music.youtube.com"):
		return time.Second // YouTube Music's web API, kept gentle (no key, no published limit)
	case has("ytimg.com"):
		return 50 * time.Millisecond // YouTube's pictures: 3 frames per video checked
	case has("googleapis.com"):
		return 200 * time.Millisecond // the YouTube Data API (its quota is counted apart)
	case has("youtube.com"):
		return time.Second // channel feeds
	case has("wikidata.org"):
		return time.Second
	}
	return 250 * time.Millisecond
}

// wait blocks until the request's host has a free slot (or the request
// ends).
func (p *hostPace) wait(r *http.Request) error {
	host := strings.ToLower(r.URL.Hostname())
	gap := p.gap(host)
	p.mu.Lock()
	if p.at == nil {
		p.at = map[string]time.Time{}
	}
	now := time.Now()
	slot := p.at[host]
	if slot.Before(now) {
		slot = now
	}
	p.at[host] = slot.Add(gap)
	if p.calls == nil {
		p.waited, p.calls = map[string]time.Duration{}, map[string]int{}
	}
	p.calls[host]++
	p.waited[host] += slot.Sub(now)
	p.mu.Unlock()
	d := time.Until(slot)
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-r.Context().Done():
		return r.Context().Err()
	case <-t.C:
		return nil
	}
}

// count adds a call to the host's numbers without a wait (calls that keep
// another pace, such as the picture pool's).
func (p *hostPace) count(r *http.Request) {
	host := strings.ToLower(r.URL.Hostname())
	p.mu.Lock()
	if p.calls == nil {
		p.calls = map[string]int{}
		p.waited = map[string]time.Duration{}
	}
	p.calls[host]++
	p.mu.Unlock()
}

// paced answers a copy of the client whose requests keep the shared pace.
func (p *hostPace) paced(c *http.Client) *http.Client {
	if c == nil {
		return nil
	}
	cp := *c
	next := cp.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	cp.Transport = pacedRT{pace: p, next: next}
	return &cp
}

type pacedRT struct {
	pace *hostPace
	next http.RoundTripper
}

func (t pacedRT) RoundTrip(r *http.Request) (*http.Response, error) {
	if err := t.pace.wait(r); err != nil {
		return nil, err
	}
	return t.next.RoundTrip(r)
}
