package main

import (
	"fmt"
	"log"
	"os"
	"runtime/debug"

	"github.com/CoderLuii/holyarrs/internal/lowmem"
)

// defaultMemoryTarget is the Go memory target when GOMEMLIMIT is not set,
// in the Docker image and standalone installs alike. It is a target, not a hard
// limit: past it Go collects garbage more often (at most about half the CPU),
// and SQLite's page cache sits outside it. GOMEMLIMIT overrides it.
const defaultMemoryTarget = 384 << 20

// memoryTarget is the target for the mode: in low-memory mode a quarter of
// the machine, between 192 and 384 MiB (256 MiB when its size is unknown).
func memoryTarget(low bool, total uint64) int64 {
	if !low {
		return defaultMemoryTarget
	}
	if total == 0 {
		return 256 << 20
	}
	return min(max(int64(total/4), 192<<20), defaultMemoryTarget)
}

// applyMemoryTarget sets target unless GOMEMLIMIT is set, and reports
// whether it did.
func applyMemoryTarget(getenv func(string) string, set func(int64) int64, target int64) bool {
	if getenv("GOMEMLIMIT") != "" {
		return false
	}
	set(target)
	return true
}

// useMemoryMode decides the low-memory mode (#95) and applies the memory
// target for the server (run), with one line saying which and why.
func useMemoryMode() {
	total := lowmem.TotalMemory("/")
	low, why, odd := lowmem.Decide(os.Getenv(lowmem.Env), total)
	lowmem.Set(low)
	if odd {
		log.Printf("holyarrs: %s=%q is not 1 or 0; deciding by the machine's size", lowmem.Env, os.Getenv(lowmem.Env))
	}
	target := memoryTarget(low, total)
	aim := "memory target from GOMEMLIMIT"
	if applyMemoryTarget(os.Getenv, debug.SetMemoryLimit, target) {
		aim = fmt.Sprintf("memory target %d MiB (set GOMEMLIMIT to change it)", target>>20)
	}
	if low {
		log.Printf("holyarrs: low-memory mode on (%s; %s=0 turns it off): smaller in-memory caches, smaller SQLite cache, %s", why, lowmem.Env, aim)
		return
	}
	log.Printf("holyarrs: low-memory mode off (%s; %s=1 turns it on), %s", why, lowmem.Env, aim)
}
