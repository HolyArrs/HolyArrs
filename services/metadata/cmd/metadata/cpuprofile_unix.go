//go:build unix

package main

import (
	"os"
	"syscall"
)

// profileSignals asks for a CPU profile (docker kill -s USR1).
func profileSignals() []os.Signal { return []os.Signal{syscall.SIGUSR1} }
