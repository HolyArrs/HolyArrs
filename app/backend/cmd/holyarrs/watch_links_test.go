package main

import "testing"

// Where a title opens on the media servers names each server and its
// address, so only people who manage the Library may ask: the owner,
// an admin and a member given manage_library get through; a member who may
// only ask is turned away before any server is read.
// route: GET /api/v1/media-servers/watch-links
func TestWatchLinksOverHTTP(t *testing.T) {
	t.Parallel()
	r := newAskRig(t)
	r.ana.call("GET", "/api/v1/media-servers/watch-links?title=1", "", 403)
	r.ana.call("GET", "/api/v1/media-servers/watch-links?title=999999", "", 403)
	r.owner.call("GET", "/api/v1/media-servers/watch-links", "", 400)
	r.owner.call("GET", "/api/v1/media-servers/watch-links?title=x", "", 400)
	r.owner.call("GET", "/api/v1/media-servers/watch-links?title=999999", "", 404)

	r.owner.call("PUT", "/api/v1/users/"+itoa(r.bo.id), `{"role":"admin"}`, 200)
	r.bo.call("GET", "/api/v1/media-servers/watch-links?title=999999", "", 404)

	r.owner.call("PUT", "/api/v1/users/"+itoa(r.ana.id), `{"perms":["ask","manage_library"]}`, 200)
	r.ana.call("GET", "/api/v1/media-servers/watch-links?title=999999", "", 404)
}
