package main

import "testing"

// The Plex label settings are the owner's and start off.
// route: GET /api/v1/settings/plex-labels
// route: PUT /api/v1/settings/plex-labels
// route: POST /api/v1/settings/plex-labels/run
func TestPlexLabelsOverHTTP(t *testing.T) {
	t.Parallel()
	r := newAskRig(t)
	r.ana.call("GET", "/api/v1/settings/plex-labels", "", 403)
	r.ana.call("PUT", "/api/v1/settings/plex-labels", `{"enabled":true}`, 403)
	r.ana.call("POST", "/api/v1/settings/plex-labels/run", "", 403)
	if b := r.owner.call("GET", "/api/v1/settings/plex-labels", "", 200); string(b) != `{"enabled":false,"label":"holyarrs","askers":false}`+"\n" {
		t.Fatalf("default %s", b)
	}
	r.owner.call("POST", "/api/v1/settings/plex-labels/run", "", 409)
	r.owner.call("PUT", "/api/v1/settings/plex-labels", `{"enabled":true,"askers":true}`, 200)
	r.owner.call("POST", "/api/v1/settings/plex-labels/run", "", 202)
	r.owner.call("PUT", "/api/v1/settings/plex-labels", `{"enabled":false}`, 200)
}
