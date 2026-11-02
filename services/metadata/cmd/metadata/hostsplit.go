package main

import (
	"net/http"
	"strings"
)

// poolForCAA answers own with the Cover Art Archive's hosts sent over the
// picture pool (its 7 addresses, each at one call a second, apart from
// MusicBrainz's paces): one address at one a second held every picture
// worker in its line (13 s waits, 2026-10-29). The archive.org file hosts
// the archive redirects to go the same way. Each pool call passes meter
// first (the upstream budget and the host pace counts, never a wait);
// other hosts keep own's client and pace; without a pool own is answered
// as is.
func poolForCAA(own, pool *http.Client, meter func(*http.Request) error) *http.Client {
	if own == nil || pool == nil {
		return own
	}
	cp := *own
	next := cp.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	pt := pool.Transport
	if pt == nil {
		pt = http.DefaultTransport
	}
	cp.Transport = hostSplit{pool: pt, own: next, meter: meter}
	return &cp
}

type hostSplit struct {
	pool, own http.RoundTripper
	meter     func(*http.Request) error
}

// poolHost reports a Cover Art Archive or archive.org host.
func poolHost(host string) bool {
	host = strings.ToLower(host)
	for _, d := range []string{"coverartarchive.org", "archive.org"} {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

func (h hostSplit) RoundTrip(r *http.Request) (*http.Response, error) {
	if poolHost(r.URL.Hostname()) {
		if h.meter != nil {
			if err := h.meter(r); err != nil {
				return nil, err
			}
		}
		return h.pool.RoundTrip(r)
	}
	return h.own.RoundTrip(r)
}
