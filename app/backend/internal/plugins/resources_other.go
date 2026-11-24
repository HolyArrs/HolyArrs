//go:build !linux

package plugins

import "runtime"

// readHostTotals: plugins run only on Linux; elsewhere memory is unknown.
func readHostTotals() hostTotals { return hostTotals{CPUs: runtime.NumCPU()} }
