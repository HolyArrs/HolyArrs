//go:build linux

package plugins

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Resource caps for a plugin's own cgroup.
const (
	cgroupPidsMax   = "256"
	cgroupCPUMax    = "100000 100000" // 1 CPU
	cgroupMemoryMax = 512 << 20
	cgroupRoot      = "/sys/fs/cgroup"
)

// cgroupBase is the app's own cgroup v2 folder with pids, memory and cpu
// enabled for its children, or "" when it is not delegated to us.
var (
	cgroupOnce    sync.Once
	cgroupBaseDir string
)

func cgroupBase() string {
	cgroupOnce.Do(func() { cgroupBaseDir = findCgroupBase() })
	return cgroupBaseDir
}

func findCgroupBase() string {
	b, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return ""
	}
	var rel string
	for _, line := range strings.Split(string(b), "\n") {
		if p, ok := strings.CutPrefix(line, "0::"); ok {
			rel = p
		}
	}
	if rel == "" {
		return "" // no cgroup v2
	}
	base := filepath.Join(cgroupRoot, filepath.Clean("/"+rel))
	want := []string{"pids", "memory", "cpu"}
	if !hasControllers(filepath.Join(base, "cgroup.subtree_control"), want) {
		if !hasControllers(filepath.Join(base, "cgroup.controllers"), want) {
			return ""
		}
		// A cgroup holding processes cannot enable controllers for children
		// (no-internal-process rule), so first move this app into a leaf of
		// its own, as a delegated service does. It fails when other processes
		// share the folder or it is not ours; then there is no usable cgroup.
		enable := func() error {
			return os.WriteFile(filepath.Join(base, "cgroup.subtree_control"), []byte("+pids +memory +cpu"), 0)
		}
		if enable() != nil {
			leaf := filepath.Join(base, "holyarrs-app")
			if err := os.Mkdir(leaf, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
				return ""
			}
			if cgroupJoin(leaf) != nil || enable() != nil {
				return ""
			}
		}
	}
	// A child we can make and remove proves the folder is delegated to us.
	probe := filepath.Join(base, "holyarrs-plugin-probe-"+randomHex())
	if os.Mkdir(probe, 0o755) != nil {
		return ""
	}
	_ = os.Remove(probe)
	return base
}

func hasControllers(file string, want []string) bool {
	b, err := os.ReadFile(file)
	if err != nil {
		return false
	}
	have := strings.Fields(string(b))
	for _, w := range want {
		found := false
		for _, h := range have {
			found = found || h == w
		}
		if !found {
			return false
		}
	}
	return true
}

// cgroupCreate makes a fresh cgroup for one start with its caps set. It
// returns "" when cgroups are not usable.
func cgroupCreate(id string, lim Limits) string {
	base := cgroupBase()
	if base == "" {
		return ""
	}
	dir := filepath.Join(base, "holyarrs-plugin-"+id+"-"+randomHex())
	if err := os.Mkdir(dir, 0o755); err != nil {
		return ""
	}
	mem := lim.MemoryMax
	if mem <= 0 {
		mem = cgroupMemoryMax
	}
	for file, v := range map[string]string{
		"pids.max":   cgroupPidsMax,
		"memory.max": strconv.FormatInt(mem, 10),
		"cpu.max":    cgroupCPUMax,
	} {
		if os.WriteFile(filepath.Join(dir, file), []byte(v), 0) != nil {
			_ = os.Remove(dir)
			return ""
		}
	}
	return dir
}

// cgroupJoin moves the calling process into dir; the helper calls it before
// the lockdown and exec.
func cgroupJoin(dir string) error {
	return os.WriteFile(filepath.Join(dir, "cgroup.procs"), []byte(strconv.Itoa(os.Getpid())), 0)
}

// cgroupKill kills everything in dir, waits for it to empty and removes it.
func cgroupKill(dir string) error {
	err := os.WriteFile(filepath.Join(dir, "cgroup.kill"), []byte("1"), 0)
	for i := 0; i < 200; i++ {
		if rmErr := os.Remove(dir); rmErr == nil || errors.Is(rmErr, os.ErrNotExist) {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err == nil {
		err = errors.New("plugin cgroup did not empty after kill")
	}
	return err
}

func randomHex() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// cgroupActive reports which caps cgroups give on this host.
func cgroupActive() ActiveLimits {
	if cgroupBase() == "" {
		return ActiveLimits{GroupKill: true}
	}
	return ActiveLimits{Cgroup: true, Pids: true, Memory: true, CPU: true, GroupKill: true}
}
