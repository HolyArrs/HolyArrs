package main

import (
	"encoding/json"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediaservers"
)

// Library stats and watch history are for people who manage the Library;
// the watch history switch is the owner's and starts off.
// route: GET /api/v1/library/stats
// route: GET /api/v1/library/stats/watched
// route: GET /api/v1/settings/watch-history
// route: PUT /api/v1/settings/watch-history
func TestLibraryStatsAndWatchHistoryOverHTTP(t *testing.T) {
	t.Parallel()
	r := newAskRig(t)
	var st mediafiles.LibraryStats
	if err := json.Unmarshal(r.owner.call("GET", "/api/v1/library/stats", "", 200), &st); err != nil || len(st.Kinds) == 0 {
		t.Fatalf("stats %+v %v", st, err)
	}
	r.ana.call("GET", "/api/v1/library/stats", "", 403)
	r.ana.call("GET", "/api/v1/library/stats/watched", "", 403)
	r.ana.call("GET", "/api/v1/settings/watch-history", "", 403)
	r.ana.call("PUT", "/api/v1/settings/watch-history", `{"enabled":true}`, 403)

	if b := r.owner.call("GET", "/api/v1/settings/watch-history", "", 200); string(b) != `{"enabled":false}`+"\n" {
		t.Fatalf("default %s", b)
	}
	r.owner.call("PUT", "/api/v1/settings/watch-history", `{}`, 400)
	r.owner.call("PUT", "/api/v1/settings/watch-history", `{"enabled":true}`, 200)
	var sum mediaservers.WatchedSummary
	if err := json.Unmarshal(r.owner.call("GET", "/api/v1/library/stats/watched", "", 200), &sum); err != nil || !sum.Enabled || sum.People == nil {
		t.Fatalf("watched %+v %v", sum, err)
	}
	r.owner.call("PUT", "/api/v1/settings/watch-history", `{"enabled":false}`, 200)
}
