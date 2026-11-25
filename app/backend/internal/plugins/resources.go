package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
)

// Default caps for one start; a manifest may ask for more, which the owner
// approves, and the owner may set each plugin's caps either way.
const (
	defaultMemoryMiB = 512
	defaultCPUs      = 1
	defaultProcesses = 256
	defaultOpenFiles = 1024
	// resourcesSince is the first app version that reads "resources".
	resourcesSince = "1.3.0"
)

// ErrResources refuses caps above what this server has.
var ErrResources = errors.New("plugin resources refused")

// Resources is what a manifest asks for, or what the owner set; a zero field
// keeps the default (or the asked value, for the owner's).
type Resources struct {
	MemoryMiB int `json:"memory_mib,omitempty"`
	CPUs      int `json:"cpus,omitempty"`
	Processes int `json:"processes,omitempty"`
	OpenFiles int `json:"open_files,omitempty"`
}

// resourceBounds are the allowed values per key, the same as the plugin
// list's own check.
var resourceBounds = map[string][2]int{
	"memory_mib": {64, 65536},
	"cpus":       {1, 64},
	"processes":  {16, 4096},
	"open_files": {64, 65536},
}

var wholeRe = regexp.MustCompile(`^-?[0-9]+$`)

var defaultResources = Resources{MemoryMiB: defaultMemoryMiB, CPUs: defaultCPUs, Processes: defaultProcesses, OpenFiles: defaultOpenFiles}

// UnmarshalJSON takes only the known keys, as whole numbers in bounds, and
// refuses an empty object.
func (r *Resources) UnmarshalJSON(b []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil || raw == nil {
		return fmt.Errorf("%w: \"resources\" must be an object", ErrManifest)
	}
	if len(raw) == 0 {
		return fmt.Errorf("%w: \"resources\" is empty", ErrManifest)
	}
	var out Resources
	for k, v := range raw {
		lim, ok := resourceBounds[k]
		if !ok {
			return fmt.Errorf("%w: unknown resources key %q", ErrManifest, k)
		}
		if !wholeRe.Match(v) {
			return fmt.Errorf("%w: resources %q must be a whole number", ErrManifest, k)
		}
		n, err := strconv.Atoi(string(v))
		if err != nil || n < lim[0] || n > lim[1] {
			return fmt.Errorf("%w: resources %q must be %d to %d", ErrManifest, k, lim[0], lim[1])
		}
		switch k {
		case "memory_mib":
			out.MemoryMiB = n
		case "cpus":
			out.CPUs = n
		case "processes":
			out.Processes = n
		case "open_files":
			out.OpenFiles = n
		}
	}
	*r = out
	return nil
}

// orDefault fills zero fields from d.
func (r Resources) orDefault(d Resources) Resources {
	if r.MemoryMiB == 0 {
		r.MemoryMiB = d.MemoryMiB
	}
	if r.CPUs == 0 {
		r.CPUs = d.CPUs
	}
	if r.Processes == 0 {
		r.Processes = d.Processes
	}
	if r.OpenFiles == 0 {
		r.OpenFiles = d.OpenFiles
	}
	return r
}

// within says whether every value of r is at most the same value of max.
func (r Resources) within(max Resources) bool {
	return r.MemoryMiB <= max.MemoryMiB && r.CPUs <= max.CPUs && r.Processes <= max.Processes && r.OpenFiles <= max.OpenFiles
}

// resourcesNeedOK says whether asked goes beyond the defaults and beyond
// what the owner approved before.
func resourcesNeedOK(asked, approved *Resources) bool {
	if asked == nil {
		return false
	}
	a := asked.orDefault(defaultResources)
	if a.within(defaultResources) {
		return false
	}
	if approved == nil {
		return true
	}
	return !a.within(approved.orDefault(defaultResources))
}

// hostTotals are this server's memory and CPUs; zero when unknown.
type hostTotals struct {
	MemoryMiB int
	CPUs      int
}

// limitsFor are a start's caps: the asked values over the defaults, the
// owner's over both, refused when above what the server has.
func limitsFor(asked, owner *Resources, host hostTotals) (Limits, error) {
	r := defaultResources
	if asked != nil {
		r = asked.orDefault(r)
	}
	if owner != nil {
		r = owner.orDefault(r)
	}
	if host.MemoryMiB > 0 && r.MemoryMiB > host.MemoryMiB {
		return Limits{}, fmt.Errorf("%w: %d MiB of memory is more than this server has (%d MiB)", ErrResources, r.MemoryMiB, host.MemoryMiB)
	}
	if host.CPUs > 0 && r.CPUs > host.CPUs {
		return Limits{}, fmt.Errorf("%w: %d CPUs is more than this server has (%d)", ErrResources, r.CPUs, host.CPUs)
	}
	return Limits{MemoryMax: int64(r.MemoryMiB) << 20, CPUs: r.CPUs, Processes: r.Processes, OpenFiles: r.OpenFiles}, nil
}

// cgroupValues are the cgroup files one start's caps write.
func cgroupValues(lim Limits) map[string]string {
	if lim.MemoryMax <= 0 {
		lim.MemoryMax = defaultMemoryMiB << 20
	}
	if lim.CPUs <= 0 {
		lim.CPUs = defaultCPUs
	}
	if lim.Processes <= 0 {
		lim.Processes = defaultProcesses
	}
	return map[string]string{
		"pids.max":   strconv.Itoa(lim.Processes),
		"memory.max": strconv.FormatInt(lim.MemoryMax, 10),
		"cpu.max":    strconv.Itoa(lim.CPUs*100000) + " 100000",
	}
}

// openFiles is a start's open-file limit.
func (l Limits) openFiles() uint64 {
	if l.OpenFiles > 0 {
		return uint64(l.OpenFiles)
	}
	return defaultOpenFiles
}
