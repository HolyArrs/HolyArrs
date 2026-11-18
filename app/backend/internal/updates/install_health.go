package updates

import (
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// How a downloaded version proves itself. Only the new version decides it
// started well, never the updater that installed it.
const (
	// HealthyAfter is how long the web server must keep answering.
	HealthyAfter = 30 * time.Second
	// Watchdog ends a start that never became healthy, so the launcher can
	// go back on the next start instead of waiting on a hang.
	Watchdog = 90 * time.Second
	// exitUnhealthy is the exit status the watchdog uses.
	exitUnhealthy = 4
)

// Run is this start of a downloaded version, from launch to healthy.
type Run struct {
	f       folder
	version string
	// FromVolume says the launcher started this program from the volume.
	FromVolume bool
	healthy    atomic.Bool
	once       sync.Once
	timer      *time.Timer
}

// StartRun begins watching this start. For the image's own program it does
// nothing. exit ends the process (nil: os.Exit).
func StartRun(configDir, version string, watchdog time.Duration, exit func(int)) *Run {
	r := &Run{f: folder(filepath.Join(configDir, AppFolder)), version: version, FromVolume: os.Getenv(envFromVolume) == "1"}
	if !r.FromVolume {
		return r
	}
	if exit == nil {
		exit = os.Exit
	}
	r.timer = time.AfterFunc(watchdog, func() {
		if !r.healthy.Load() {
			log.Printf("updates: %s did not become healthy in %s, stopping so the previous version comes back", version, watchdog)
			exit(exitUnhealthy)
		}
	})
	return r
}

// Ready says the database is open, migrations are done and the web server
// listens; after HealthyAfter more the start counts as healthy.
func (r *Run) Ready(after time.Duration, answering func() bool) {
	if !r.FromVolume {
		return
	}
	time.AfterFunc(after, func() {
		if answering == nil || answering() {
			r.MarkHealthy()
		}
	})
}

// MarkHealthy ends the trial: the start record is removed.
func (r *Run) MarkHealthy() {
	r.once.Do(func() {
		r.healthy.Store(true)
		if r.timer != nil {
			r.timer.Stop()
		}
		var p pending
		if r.f.read(pendingFile, &p) && p.Version == r.version {
			r.f.remove(pendingFile)
		}
		log.Printf("updates: %s started well", r.version)
	})
}

// Healthy says this start proved itself (always true for the image).
func (r *Run) Healthy() bool { return !r.FromVolume || r.healthy.Load() }

// CleanStop is a stop asked for before healthy (SIGTERM, a restart): it is
// not a failed start, so the count goes back down.
func (r *Run) CleanStop() {
	if !r.FromVolume || r.healthy.Load() {
		return
	}
	var p pending
	if r.f.read(pendingFile, &p) && p.Version == r.version && p.Starts > 0 {
		p.Starts--
		_ = r.f.write(pendingFile, p)
		log.Printf("updates: stopped before %s proved itself; not counted as a failed start", r.version)
	}
}
