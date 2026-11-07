package startup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDecideLowMemory(t *testing.T) {
	const gib = 1 << 30
	for _, c := range []struct {
		env   string
		total uint64
		want  bool
		odd   bool
	}{
		{"", 0, false, false},
		{"", gib, true, false},
		{"", 2 * gib, true, false},
		{"", LowMemoryAutoAtOrBelow, true, false},
		{"", 3700 << 20, false, false},
		{"1", 8 * gib, true, false},
		{" ON ", 0, true, false},
		{"true", 8 * gib, true, false},
		{"0", gib, false, false},
		{"off", gib, false, false},
		{"no", 0, false, false},
		{"banana", gib, true, true},
		{"banana", 8 * gib, false, true},
	} {
		got, why, odd := DecideLowMemory(c.env, c.total)
		if got != c.want || odd != c.odd || why == "" {
			t.Errorf("DecideLowMemory(%q, %d) = %v %q odd %v, want %v odd %v", c.env, c.total, got, why, odd, c.want, c.odd)
		}
	}
}

func TestPick(t *testing.T) {
	t.Cleanup(func() { SetLowMemory(false) })
	if LowMemoryPick(1000, 150) != 1000 {
		t.Fatal("off: not the normal value")
	}
	SetLowMemory(true)
	if LowMemoryPick(1000, 150) != 150 || !LowMemoryOn() {
		t.Fatal("on: not the low value")
	}
}

// fake writes files under a fresh root.
func fake(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for p, body := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestTotalMemory(t *testing.T) {
	const meminfo = "MemTotal:        1918020 kB\nMemFree:  100 kB\n"
	const total = 1918020 << 10
	for _, c := range []struct {
		name  string
		files map[string]string
		want  uint64
	}{
		{"no files", nil, 0},
		{"meminfo only", map[string]string{"proc/meminfo": meminfo}, total},
		{"garbage meminfo", map[string]string{"proc/meminfo": "MemTotal: lots\n"}, 0},
		{"v2 no limit", map[string]string{"proc/meminfo": meminfo, "proc/self/cgroup": "0::/\n", "sys/fs/cgroup/memory.max": "max\n"}, total},
		{"v2 limit", map[string]string{"proc/meminfo": meminfo, "proc/self/cgroup": "0::/\n", "sys/fs/cgroup/memory.max": "1073741824\n"}, 1 << 30},
		{"v2 parent smaller", map[string]string{
			"proc/meminfo": meminfo, "proc/self/cgroup": "0::/system.slice/docker-abc.scope\n",
			"sys/fs/cgroup/system.slice/docker-abc.scope/memory.max": "max",
			"sys/fs/cgroup/system.slice/memory.max":                  "536870912",
		}, 512 << 20},
		{"v1 unlimited", map[string]string{"proc/meminfo": meminfo, "proc/self/cgroup": "5:cpuacct,memory:/docker/abc\n", "sys/fs/cgroup/memory/docker/abc/memory.limit_in_bytes": "9223372036854771712"}, total},
		{"v1 limit", map[string]string{"proc/meminfo": meminfo, "proc/self/cgroup": "garbage\n4:memory:/docker/abc\n", "sys/fs/cgroup/memory/docker/abc/memory.limit_in_bytes": "536870912"}, 512 << 20},
		{"limit above MemTotal", map[string]string{"proc/meminfo": meminfo, "proc/self/cgroup": "0::/\n", "sys/fs/cgroup/memory.max": "68719476736"}, total},
		{"bad limit", map[string]string{"proc/meminfo": meminfo, "proc/self/cgroup": "0::/\n", "sys/fs/cgroup/memory.max": "12x"}, total},
	} {
		root := fake(t, c.files)
		if got := TotalMemory(root); got != c.want {
			t.Errorf("%s: %d, want %d", c.name, got, c.want)
		}
	}
}
