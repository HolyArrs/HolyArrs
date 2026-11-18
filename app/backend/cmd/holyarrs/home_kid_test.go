package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
)

// route: GET /api/v1/explore/head
// Home before setup borrows Explore's first rows with the same request
// Home sends (type=all with the viewer's time zone). A Kid's answer is the
// Explore one, filtered by their age limit: no title over it, hero included.
func TestKidHomePicturesStayUnderLimit(t *testing.T) {
	k := newKidRig(t)
	// Explore's pools read the same fake TMDB as the rest of the rig.
	k.api.explore.Paced = &engine.Paced{Source: discover.ListerSource{L: k.tmdb.provider()}, Gap: time.Millisecond}
	const home = "/api/v1/explore/head?type=all&tz=America%2FNew_York"
	// Wait for the rows to be built, then check the same rows for a member without a limit hold titles over PG, so the Kid check has something to catch.
	var all string
	for end := time.Now().Add(20 * time.Second); time.Now().Before(end); time.Sleep(200 * time.Millisecond) {
		_, all = k.ana.get(home)
		_, _ = k.kai.get(home)
		for key, ok := k.api.explore.NextDue(); ok; key, ok = k.api.explore.NextDue() {
			if err := k.api.explore.BuildPool(context.Background(), key.Member, key.Switch); err != nil {
				t.Fatal(err)
			}
		}
		if !strings.Contains(string(all), `"building":true`) && mentionsBlocked(string(all)) != "" {
			break
		}
	}
	if mentionsBlocked(string(all)) == "" {
		t.Fatalf("control: the unlimited rows hold no title over PG: %s", all)
	}
	code, b := k.kai.get(home)
	if code != 200 && code != 403 {
		t.Fatalf("kid head %d", code)
	}
	if hit := mentionsBlocked(string(b)); hit != "" {
		t.Fatalf("kid sees %s on Home's rows: %s", hit, b)
	}
}
