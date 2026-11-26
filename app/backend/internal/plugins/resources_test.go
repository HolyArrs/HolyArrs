package plugins

import (
	"errors"
	"strings"
	"testing"
)

func TestManifestResources(t *testing.T) {
	base := `{"id":"x","name":"X","version":"1.0.0","author":"a","min_app_version":"%V","sha256":"` + strings.Repeat("a", 64) + `","access":{"hosts":[],"folders":[],"actions":[]}%R}`
	for _, c := range []struct {
		name, ver, res string
		ok             bool
	}{
		{"none", "1.0.0", ``, true},
		{"all", "1.3.0", `,"resources":{"memory_mib":1024,"cpus":2,"processes":512,"open_files":4096}`, true},
		{"one", "1.3.0", `,"resources":{"memory_mib":64}`, true},
		{"top bounds", "1.3.0", `,"resources":{"memory_mib":65536,"cpus":64,"processes":4096,"open_files":65536}`, true},
		{"low bounds", "1.3.0", `,"resources":{"memory_mib":64,"cpus":1,"processes":16,"open_files":64}`, true},
		{"newer app", "1.4.0", `,"resources":{"cpus":2}`, true},
		{"old app", "1.2.9", `,"resources":{"cpus":2}`, false},
		{"pre-release app", "1.3.0-rc.1", `,"resources":{"cpus":2}`, false},
		{"empty", "1.3.0", `,"resources":{}`, false},
		{"unknown key", "1.3.0", `,"resources":{"gpus":1}`, false},
		{"case variant", "1.3.0", `,"resources":{"CPUs":2}`, false},
		{"fraction", "1.3.0", `,"resources":{"cpus":1.5}`, false},
		{"bool", "1.3.0", `,"resources":{"cpus":true}`, false},
		{"string", "1.3.0", `,"resources":{"cpus":"2"}`, false},
		{"null", "1.3.0", `,"resources":{"cpus":null}`, false},
		{"memory low", "1.3.0", `,"resources":{"memory_mib":63}`, false},
		{"memory high", "1.3.0", `,"resources":{"memory_mib":65537}`, false},
		{"cpus low", "1.3.0", `,"resources":{"cpus":0}`, false},
		{"cpus high", "1.3.0", `,"resources":{"cpus":65}`, false},
		{"processes low", "1.3.0", `,"resources":{"processes":15}`, false},
		{"processes high", "1.3.0", `,"resources":{"processes":4097}`, false},
		{"files low", "1.3.0", `,"resources":{"open_files":63}`, false},
		{"files high", "1.3.0", `,"resources":{"open_files":65537}`, false},
		{"not an object", "1.3.0", `,"resources":[]`, false},
	} {
		raw := strings.NewReplacer("%V", c.ver, "%R", c.res).Replace(base)
		_, err := ParseManifest([]byte(raw))
		if (err == nil) != c.ok {
			t.Errorf("%s: err %v, want ok %v", c.name, err, c.ok)
		}
	}
}

func TestLimitsFor(t *testing.T) {
	host := hostTotals{MemoryMiB: 8192, CPUs: 4}
	for _, c := range []struct {
		name    string
		asked   *Resources
		owner   *Resources
		want    Limits
		refused bool
	}{
		{"defaults", nil, nil, Limits{MemoryMax: 512 << 20, CPUs: 1, Processes: 256, OpenFiles: 1024}, false},
		{"asked", &Resources{MemoryMiB: 2048, CPUs: 2}, nil, Limits{MemoryMax: 2048 << 20, CPUs: 2, Processes: 256, OpenFiles: 1024}, false},
		{"owner wins", &Resources{MemoryMiB: 2048, CPUs: 2}, &Resources{MemoryMiB: 1024, Processes: 64}, Limits{MemoryMax: 1024 << 20, CPUs: 2, Processes: 64, OpenFiles: 1024}, false},
		{"owner raises", nil, &Resources{CPUs: 3}, Limits{MemoryMax: 512 << 20, CPUs: 3, Processes: 256, OpenFiles: 1024}, false},
		{"above memory", &Resources{MemoryMiB: 16384}, nil, Limits{}, true},
		{"above cpus", &Resources{CPUs: 8}, nil, Limits{}, true},
		{"owner above", nil, &Resources{MemoryMiB: 9000}, Limits{}, true},
	} {
		got, err := limitsFor(c.asked, c.owner, host)
		if c.refused {
			if !errors.Is(err, ErrResources) {
				t.Errorf("%s: err %v, want refused", c.name, err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%s: %+v %v, want %+v", c.name, got, err, c.want)
		}
	}
	// Unknown totals (not Linux) skip the ceiling.
	if _, err := limitsFor(&Resources{MemoryMiB: 65536}, nil, hostTotals{}); err != nil {
		t.Errorf("unknown totals: %v", err)
	}
}

// The values a start's cgroup gets.
func TestCgroupValues(t *testing.T) {
	for _, c := range []struct {
		lim  Limits
		want map[string]string
	}{
		{Limits{}, map[string]string{"pids.max": "256", "memory.max": "536870912", "cpu.max": "100000 100000"}},
		{Limits{MemoryMax: 1 << 30, CPUs: 2, Processes: 512}, map[string]string{"pids.max": "512", "memory.max": "1073741824", "cpu.max": "200000 100000"}},
	} {
		got := cgroupValues(c.lim)
		for k, v := range c.want {
			if got[k] != v {
				t.Errorf("%+v %s = %q want %q", c.lim, k, got[k], v)
			}
		}
	}
}

func TestResourcesNeedApproval(t *testing.T) {
	m := &Manifest{Resources: &Resources{MemoryMiB: 2048}}
	if d := accessDiff(m, Approval{}); len(d) != 1 || d[0] != "resources" {
		t.Fatalf("diff %v", d)
	}
	if d := accessDiff(m, Approval{Resources: &Resources{MemoryMiB: 2048}}); len(d) != 0 {
		t.Fatalf("approved diff %v", d)
	}
	m.Resources = &Resources{MemoryMiB: 2048, CPUs: 2}
	if d := accessDiff(m, Approval{Resources: &Resources{MemoryMiB: 2048}}); len(d) != 1 {
		t.Fatalf("more asked diff %v", d)
	}
	m.Resources = &Resources{MemoryMiB: 256}
	if d := accessDiff(m, Approval{}); len(d) != 0 {
		t.Fatalf("below defaults diff %v", d)
	}
}

func TestManifestActionsNeedApp130(t *testing.T) {
	base := `{"id":"x","name":"X","version":"1.0.0","author":"a","min_app_version":"%V","sha256":"` + strings.Repeat("a", 64) + `","access":{"hosts":[],"folders":[],"actions":[%A]}}`
	for _, c := range []struct {
		name, ver, act string
		ok             bool
	}{
		{"no actions on an old app", "1.0.0", ``, true},
		{"tailscale forward on 1.3.0", "1.3.0", `"network.forward"`, true},
		{"scope on 1.4.0", "1.4.0", `"library.read"`, true},
		{"scope on 1.2.1", "1.2.1", `"library.read"`, false},
		{"unknown scope", "1.3.0", `"library.everything"`, false},
		{"closed area", "1.3.0", `"plugins.change"`, false},
		{"repeated", "1.3.0", `"library.read","library.read"`, false},
	} {
		_, err := ParseManifest([]byte(strings.NewReplacer("%V", c.ver, "%A", c.act).Replace(base)))
		if (err == nil) != c.ok {
			t.Errorf("%s: %v", c.name, err)
		}
	}
}
