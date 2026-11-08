package startup

import (
	"bufio"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
)

// LowMemoryEnv is the setting's name.
const LowMemoryEnv = "HOLYARRS_LOW_MEMORY"

// LowMemoryAutoAtOrBelow turns the mode on by itself: a 2 GB machine reports about
// 1.8-1.9 GiB, a container limited to 2g exactly 2 GiB, a 4 GB machine 3.7 GiB.
const LowMemoryAutoAtOrBelow = 2560 << 20

var on atomic.Bool

// LowMemoryOn says the mode is on.
func LowMemoryOn() bool { return on.Load() }

// SetLowMemory turns the mode on or off (at start, and in tests).
func SetLowMemory(v bool) { on.Store(v) }

// LowMemoryPick is normal, or low while the mode is on.
func LowMemoryPick(normal, low int) int {
	if on.Load() {
		return low
	}
	return normal
}

// DecideLowMemory reads the setting: 1/true/on/yes and 0/false/off/no win; anything
// else (or nothing) leaves it to the machine's size, where 0 means unknown
// and keeps the mode off. why says which, for the start-up line; odd is a
// value that was neither and was read as unset.
func DecideLowMemory(env string, total uint64) (lowOn bool, why string, odd bool) {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "1", "true", "on", "yes":
		return true, LowMemoryEnv + "=1", false
	case "0", "false", "off", "no":
		return false, LowMemoryEnv + "=0", false
	case "":
	default:
		odd = true
	}
	if total == 0 {
		return false, "machine size unknown", odd
	}
	return total <= LowMemoryAutoAtOrBelow, LowMemorySize(total), odd
}

// LowMemorySize writes a byte count as GiB with one decimal ("1.8 GiB").
func LowMemorySize(n uint64) string {
	return strconv.FormatFloat(float64(n)/(1<<30), 'f', 1, 64) + " GiB"
}

// TotalMemory is the memory this process may use: the machine's MemTotal or
// a smaller cgroup (v1 or v2) limit, read under root ("/"). 0 when it cannot
// be read (not Linux, or an odd system): every read error is skipped.
func TotalMemory(root string) uint64 {
	total := memTotal(filepath.Join(root, "proc", "meminfo"))
	if total == 0 {
		return 0
	}
	if lim := cgroupLimit(root); lim > 0 && lim < total {
		return lim
	}
	return total
}

func memTotal(path string) uint64 {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		rest, ok := strings.CutPrefix(sc.Text(), "MemTotal:")
		if !ok {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			return 0
		}
		kb, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			return 0
		}
		return kb << 10
	}
	return 0
}

// cgroupLimit is the smallest memory limit on this process's cgroup path and
// its parents (v2 memory.max, v1 memory.limit_in_bytes); 0 for none.
func cgroupLimit(root string) uint64 {
	raw, err := os.ReadFile(filepath.Join(root, "proc", "self", "cgroup"))
	if err != nil {
		return 0
	}
	var best uint64
	keep := func(n uint64) {
		if n > 0 && n < 1<<62 && (best == 0 || n < best) {
			best = n
		}
	}
	for _, line := range strings.Split(string(raw), "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), ":", 3)
		if len(parts) != 3 {
			continue
		}
		switch {
		case parts[0] == "0" && parts[1] == "":
			walk(filepath.Join(root, "sys", "fs", "cgroup"), parts[2], "memory.max", keep)
		case hasMemory(parts[1]):
			walk(filepath.Join(root, "sys", "fs", "cgroup", "memory"), parts[2], "memory.limit_in_bytes", keep)
		}
	}
	return best
}

func hasMemory(controllers string) bool {
	for _, c := range strings.Split(controllers, ",") {
		if c == "memory" {
			return true
		}
	}
	return false
}

// walk reads file in base/path and each parent up to base ("max" and
// unreadable values are skipped).
func walk(base, cg, file string, keep func(uint64)) {
	p := path.Clean("/" + cg) // a cgroup path always uses slashes
	for {
		if raw, err := os.ReadFile(filepath.Join(base, filepath.FromSlash(p), file)); err == nil {
			if n, err := strconv.ParseUint(strings.TrimSpace(string(raw)), 10, 64); err == nil {
				keep(n)
			}
		}
		if p == "/" {
			return
		}
		p = path.Dir(p)
	}
}
