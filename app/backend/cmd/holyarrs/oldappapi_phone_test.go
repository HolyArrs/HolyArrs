package main

import (
	"fmt"
	"strings"
	"testing"
)

// The deeper calls phone apps such as Helmarr and nzb360 make, through the
// whole app: the key goes in X-Api-Key, each side's key reaches only its
// own side, and no key reaches nothing.
func TestOldAppPhoneCalls(t *testing.T) {
	t.Parallel()
	f := newOldAppFix(t)
	f.turnOn()
	show := f.addLabShow()
	movie := f.addLabMovie()
	ep := f.itemOf(show, 1, 1)
	s, r := "/sonarr/api/v3/", "/radarr/api/v3/"

	// One episode and its monitor switch: only monitored is applied.
	st, b := f.pulsarr(f.sonarrKey, "GET", s+"episode/"+arrItoa(ep), "")
	if st != 200 || !strings.Contains(b, `"monitored":true`) {
		t.Fatalf("episode: %d %s", st, b)
	}
	st, b = f.pulsarr(f.sonarrKey, "PUT", s+"episode/"+arrItoa(ep), fmt.Sprintf(`{"id":%d,"seriesId":%d,"title":"renamed","monitored":false}`, ep, show))
	if st != 202 || !strings.Contains(b, `"monitored":false`) || strings.Contains(b, "renamed") {
		t.Fatalf("PUT episode: %d %s", st, b)
	}
	if got := f.keptEpisodes(show); strings.Contains(strings.Join(got, ","), "S1E1") {
		t.Fatalf("still kept after PUT: %v", got)
	}

	routes := []struct{ side, key, method, path, body string }{
		{"sonarr", f.sonarrKey, "GET", s + "episode/" + arrItoa(ep), ""},
		{"sonarr", f.sonarrKey, "PUT", s + "episode/" + arrItoa(ep), `{"monitored":true}`},
		{"sonarr", f.sonarrKey, "GET", s + "history/series?seriesId=" + arrItoa(show), ""},
		{"sonarr", f.sonarrKey, "GET", s + "wanted/cutoff", ""},
		{"sonarr", f.sonarrKey, "GET", s + "release", ""},
		{"sonarr", f.sonarrKey, "POST", s + "release", `{"guid":"holyarrs-release-1","indexerId":1}`},
		{"sonarr", f.sonarrKey, "DELETE", s + "queue/1000", ""},
		{"sonarr", f.sonarrKey, "GET", s + "blocklist", ""},
		{"sonarr", f.sonarrKey, "GET", s + "system/task", ""},
		{"sonarr", f.sonarrKey, "GET", s + "log", ""},
		{"sonarr", f.sonarrKey, "GET", s + "update", ""},
		{"radarr", f.radarrKey, "GET", r + "history/movie?movieId=" + arrItoa(movie), ""},
		{"radarr", f.radarrKey, "GET", r + "moviefile?movieId=" + arrItoa(movie), ""},
		{"radarr", f.radarrKey, "GET", r + "wanted/cutoff", ""},
		{"radarr", f.radarrKey, "GET", r + "release", ""},
		{"radarr", f.radarrKey, "GET", r + "blocklist", ""},
		{"radarr", f.radarrKey, "GET", r + "system/task", ""},
	}
	for _, c := range routes {
		st, b := f.pulsarr(c.key, c.method, c.path, c.body)
		if st == 401 || st == 403 || st == 404 && !strings.Contains(c.path, "queue/") && c.method != "POST" || st >= 500 {
			t.Fatalf("%s %s: %d %s", c.method, c.path, st, b)
		}
		other := f.radarrKey
		if c.side == "radarr" {
			other = f.sonarrKey
		}
		if st, b := f.pulsarr(other, c.method, c.path, c.body); st != 401 && st != 403 {
			t.Fatalf("%s %s with the other side's key: %d %s", c.method, c.path, st, b)
		}
		if st, b := f.pulsarr("", c.method, c.path, c.body); st != 401 && st != 403 {
			t.Fatalf("%s %s with no key: %d %s", c.method, c.path, st, b)
		}
	}
}
