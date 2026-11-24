package plugins

import (
	"bufio"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// readHostTotals reads this server's memory and CPUs.
func readHostTotals() hostTotals {
	h := hostTotals{CPUs: runtime.NumCPU()}
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return h
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if rest, ok := strings.CutPrefix(sc.Text(), "MemTotal:"); ok {
			kb, _ := strconv.Atoi(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(rest), "kB")))
			h.MemoryMiB = kb / 1024
		}
	}
	return h
}
