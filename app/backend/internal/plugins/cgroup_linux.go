//go:build linux

package plugins

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// cgroupRoot is where cgroup v2 is mounted; a start's caps: cgroupValues.
const cgroupRoot = "/sys/fs/cgroup"

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
			// Only move when this app is alone in the folder.
			procs, err := os.ReadFile(filepath.Join(base, "cgroup.procs"))
			if err != nil || strings.TrimSpace(string(procs)) != strconv.Itoa(os.Getpid()) {
				return ""
			}
			leaf := filepath.Join(base, "holyarrs-app")
			if err := os.Mkdir(leaf, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
				return ""
			}
			if cgroupJoin(leaf) != nil {
				_ = os.Remove(leaf)
				return ""
			}
			if enable() != nil {
				_ = cgroupJoin(base)
				_ = os.Remove(leaf)
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
// returns "" when cgroups are not usable, and an error when they are but
// this one could not be made.
func cgroupCreate(id string, lim Limits) (string, error) {
	base := cgroupBase()
	if base == "" {
		return "", nil
	}
	dir := filepath.Join(base, "holyarrs-plugin-"+id+"-"+randomHex())
	if err := os.Mkdir(dir, 0o755); err != nil {
		return "", fmt.Errorf("plugin cgroup: %w", err)
	}
	for file, v := range cgroupValues(lim) {
		if err := os.WriteFile(filepath.Join(dir, file), []byte(v), 0); err != nil {
			_ = os.Remove(dir)
			return "", fmt.Errorf("plugin cgroup %s: %w", file, err)
		}
	}
	return dir, nil
}

// cgroupJoin moves the calling process into dir; the helper calls it before
// the lockdown and exec.
func cgroupJoin(dir string) error {
	return os.WriteFile(filepath.Join(dir, "cgroup.procs"), []byte(strconv.Itoa(os.Getpid())), 0)
}

// cgroupKill kills everything in dir, waits for it to empty and removes it.
// Without a working cgroup.kill it freezes the cgroup and kills each process.
func cgroupKill(dir string) error {
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	var err error
	if os.WriteFile(filepath.Join(dir, "cgroup.kill"), []byte("1"), 0) != nil {
		err = cgroupKillEach(dir)
	}
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

// cgroupKillEach freezes dir and SIGKILLs every process in it until it is
// empty or about 2s pass.
func cgroupKillEach(dir string) error {
	_ = os.WriteFile(filepath.Join(dir, "cgroup.freeze"), []byte("1"), 0)
	for i := 0; i < 200; i++ {
		b, err := os.ReadFile(filepath.Join(dir, "cgroup.procs"))
		if err != nil {
			return fmt.Errorf("plugin cgroup procs: %w", err)
		}
		pids := strings.Fields(string(b))
		if len(pids) == 0 {
			return nil
		}
		for _, p := range pids {
			if pid, err := strconv.Atoi(p); err == nil {
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	return errors.New("plugin cgroup did not empty after killing each process")
}

func randomHex() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// cgroupActive reports which caps cgroups give on this host. They stay on
// without Landlock too (a plugin the owner let run without the file lock),
// where they still cap it and stop its whole group.
func cgroupActive() ActiveLimits {
	if !cgroupUsable() {
		return ActiveLimits{GroupKill: true}
	}
	return ActiveLimits{Cgroup: true, Pids: true, Memory: true, CPU: true, GroupKill: true}
}

// cgroupUsable is true when the delegated cgroup has cgroup.kill, so a start
// gets caps and a whole-group kill.
func cgroupUsable() bool {
	base := cgroupBase()
	if base == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(base, "cgroup.kill"))
	return err == nil
}
