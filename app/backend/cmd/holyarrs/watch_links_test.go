package main

import "testing"

// Where a title opens on the media servers names each server and its
// address, so only people who manage the Library may ask.
// route: GET /api/v1/media-servers/watch-links
func TestWatchLinksOverHTTP(t *testing.T) {
	t.Parallel()
	r := newAskRig(t)
	r.ana.call("GET", "/api/v1/media-servers/watch-links?title=1", "", 403)
	r.owner.call("GET", "/api/v1/media-servers/watch-links", "", 400)
	r.owner.call("GET", "/api/v1/media-servers/watch-links?title=x", "", 400)
	r.owner.call("GET", "/api/v1/media-servers/watch-links?title=999999", "", 404)
}
