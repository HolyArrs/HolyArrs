//go:build lab

package migrate

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
)

// The search against the lab's real apps on 127.0.0.1 (their lab ports in
// place of the usual ones). HOLYARRS_LAB_FIND_PORTS is kind=port,kind=port.
func TestLabFind(t *testing.T) {
	spec := os.Getenv("HOLYARRS_LAB_FIND_PORTS")
	if spec == "" {
		t.Skip("HOLYARRS_LAB_FIND_PORTS is not set")
	}
	var targets []findTarget
	want := map[Kind]bool{}
	for _, kv := range strings.Split(spec, ",") {
		k, p, _ := strings.Cut(kv, "=")
		n, _ := strconv.Atoi(p)
		targets = append(targets, findTarget{Host: "127.0.0.1", Port: n, Kinds: []Kind{Kind(k)}})
		want[Kind(k)] = true
	}
	// A closed lab port is skipped.
	targets = append(targets, findTarget{Host: "127.0.0.1", Port: 18747, Kinds: []Kind{Lidarr}})
	old := findTargets
	t.Cleanup(func() { findTargets = old })
	findTargets = func([]string) []findTarget { return targets }
	got, _ := Find(context.Background(), nil)
	for _, f := range got {
		t.Logf("found %s %s %s", f.Kind, f.Address, f.Version)
		delete(want, f.Kind)
	}
	if len(want) > 0 {
		t.Errorf("not found: %v", want)
	}
}

// The apps a lab Overseerr, Seerr or Prowlarr is set up with. Keys come
// from HOLYARRS_LAB_<APP>_KEY and are never logged.
func TestLabLinked(t *testing.T) {
	for _, k := range []Kind{Overseerr, Seerr, Prowlarr} {
		up := strings.ToUpper(string(k))
		u, key := os.Getenv("HOLYARRS_LAB_"+up+"_URL"), os.Getenv("HOLYARRS_LAB_"+up+"_KEY")
		if u == "" || key == "" {
			t.Logf("%s: not set", k)
			continue
		}
		apps, err := ReadLinked(context.Background(), api(k, u, key))
		if err != nil {
			t.Errorf("%s: %v", k, err)
			continue
		}
		t.Logf("%s: %d linked", k, len(apps))
		for _, a := range apps {
			t.Logf("  %s %q %s key=%v reached=%v %s", a.Kind, a.Name, a.Address, a.HasKey, a.Reachable, a.Version)
		}
	}
}
