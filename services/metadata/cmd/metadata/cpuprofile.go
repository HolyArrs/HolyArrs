package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"runtime/pprof"
	"sort"
	"time"
)

// ProfileEnv turns the SIGUSR1 CPU profiler on ("1"); it is off unless set.
const ProfileEnv = "HOLYARRS_CPU_PROFILE"

// profileKeep is how many profile files are kept; older ones are removed.
const profileKeep = 3

// cpuProfileOn writes a CPU profile of length d to dir/cpu-<unix>.pprof
// each time ask fires (SIGUSR1 in serve with HOLYARRS_CPU_PROFILE=1;
// nothing listens on the network). One at a time; a second ask while one
// runs is dropped. Only the newest profileKeep files stay.
func cpuProfileOn(ctx context.Context, ask <-chan os.Signal, dir string, d time.Duration) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-ask:
		}
		p := filepath.Join(dir, "cpu-"+time.Now().UTC().Format("20060102T150405")+".pprof")
		f, err := os.Create(p)
		if err != nil {
			log.Printf("cpu profile: %v", err)
			continue
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			f.Close()
			log.Printf("cpu profile: %v", err)
			continue
		}
		select {
		case <-ctx.Done():
		case <-time.After(d):
		}
		pprof.StopCPUProfile()
		f.Close()
		log.Printf("cpu profile: wrote %s", p)
		pruneProfiles(dir, profileKeep)
	}
}

// pruneProfiles keeps the newest keep cpu-*.pprof files in dir (their
// names sort by time).
func pruneProfiles(dir string, keep int) {
	m, _ := filepath.Glob(filepath.Join(dir, "cpu-*.pprof"))
	sort.Strings(m)
	for len(m) > keep {
		_ = os.Remove(m[0])
		m = m[1:]
	}
}
