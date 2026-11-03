package main

import (
	"log"
	"os"
	"runtime/debug"
)

// defaultMemoryTarget is the Go memory target when GOMEMLIMIT is not set:
// the value the Docker image and the install notes already use, so a
// Windows or standalone install behaves the same. It is a target, not a hard
// limit: past it Go collects garbage more often (at most about half the CPU),
// and SQLite's page cache sits outside it. GOMEMLIMIT overrides it.
const defaultMemoryTarget = 384 << 20

// applyMemoryTarget sets the default memory target unless GOMEMLIMIT is set,
// and reports whether it did.
func applyMemoryTarget(getenv func(string) string, set func(int64) int64) bool {
	if getenv("GOMEMLIMIT") != "" {
		return false
	}
	set(defaultMemoryTarget)
	return true
}

// useMemoryTarget applies the default target for the server (run).
func useMemoryTarget() {
	if applyMemoryTarget(os.Getenv, debug.SetMemoryLimit) {
		log.Printf("holyarrs: memory target %d MiB (set GOMEMLIMIT to change it)", defaultMemoryTarget>>20)
	}
}
