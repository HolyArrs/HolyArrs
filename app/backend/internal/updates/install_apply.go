package updates

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CheckCommand is the read-only dry run a new program passes before the
// switch: `holyarrs update-check -config <dir>`.
const CheckCommand = "update-check"

// Install limits.
const (
	downloadTimeout = 10 * time.Minute
	// IdleWait is how long an update waits for imports and moves to finish.
	IdleWait = 10 * time.Minute
	idlePoll = 5 * time.Second
)

// Install phases, shown while an update runs.
const (
	PhaseIdle        = "idle"
	PhaseDownloading = "downloading"
	PhaseChecking    = "checking"
	PhaseReady       = "ready"
	PhaseWaiting     = "waiting"
	PhaseCopying     = "copying"
	PhaseRestarting  = "restarting"
	PhaseFailed      = "failed"
)

// Installer downloads, checks and switches to a newer version in the
// config folder. The image's program is never changed.
type Installer struct {
	ConfigDir string
	Version   string
	// Keys check signatures (nil: Pinned()).
	Keys []PublicKey
	// Client downloads (tests); nil uses a client limited to allowed hosts.
	Client *http.Client
	// Check replaces the dry run (tests).
	Check func(ctx context.Context, bin string) error
	// Snapshot copies the databases into dir before a release that
	// changes them.
	Snapshot func(dir string) error
	// Busy says what still runs that an update must wait for ("" when idle).
	Busy func() string
	// Restart stops the app cleanly and starts the launcher again.
	Restart func()
	// IdleWait replaces IdleWait (tests).
	IdleWait time.Duration

	mu         sync.Mutex
	installing bool
	preparing  string
	prog       Progress
	staged     *Installed
	offer      Channel
}

// Progress is the install's current step.
type Progress struct {
	Phase   string    `json:"phase"`
	Version string    `json:"version,omitempty"`
	Message string    `json:"message,omitempty"`
	At      time.Time `json:"at"`
}

// ErrNotReady says there is no checked download to install.
var ErrNotReady = errors.New("no update is downloaded and checked yet")

// Prepare downloads and checks the offered release so it is ready to
// install. It keeps the signed list for the launcher's withdrawn check.
func (in *Installer) Prepare(ctx context.Context, l *List, data, sig []byte, c Channel) error {
	if NoUpdater {
		return errors.New("this build is updated by its package manager")
	}
	f := in.folder()
	if err := os.MkdirAll(string(f), 0o750); err != nil {
		return err
	}
	if data != nil {
		_ = writeAtomic(f.path(keptListFile+SigSuffix), sig, 0o600)
		_ = writeAtomic(f.path(keptListFile), data, 0o600)
	}
	if why := whyNot(l, c, in.Version, f.skip()); why != "" {
		return errors.New(why)
	}
	in.mu.Lock()
	if (in.staged != nil && in.staged.Version == c.Version) || in.installing {
		in.mu.Unlock()
		return nil
	}
	if in.preparing != "" {
		in.mu.Unlock()
		return errors.New("a download is already running")
	}
	in.preparing = c.Version
	in.mu.Unlock()
	got, err := in.stage(ctx, c)
	in.mu.Lock()
	in.preparing = ""
	if err == nil {
		in.staged, in.offer = &got, c
	}
	in.mu.Unlock()
	if err != nil {
		in.progress(PhaseFailed, c.Version, err.Error())
		return err
	}
	in.progress(PhaseReady, c.Version, "")
	return nil
}

// Staged is the downloaded, checked release, if any.
func (in *Installer) Staged() (Channel, bool) {
	in.mu.Lock()
	defer in.mu.Unlock()
	return in.offer, in.staged != nil
}

// Install waits until nothing important runs, copies the database when the
// release changes it, points the launcher at the new version and restarts.
func (in *Installer) Install(ctx context.Context) (err error) {
	in.mu.Lock()
	st := in.staged
	if st == nil || in.installing || in.preparing != "" {
		in.mu.Unlock()
		if st == nil {
			return ErrNotReady
		}
		return nil // already installing: a second click changes nothing
	}
	// Stays set after a restart is asked for; cleared only on failure.
	in.installing = true
	in.mu.Unlock()
	defer func() {
		if err != nil {
			in.mu.Lock()
			in.installing = false
			in.mu.Unlock()
		}
	}()
	if err := in.waitIdle(ctx, st.Version); err != nil {
		in.progress(PhaseFailed, st.Version, err.Error())
		return err
	}
	f := in.folder()
	// A release that keeps the database leaves an earlier copy alone:
	// going back past the release that made it still needs it.
	if st.Migrates {
		in.progress(PhaseCopying, st.Version, "")
		if err := in.copyDB(f, st.Version); err != nil {
			in.progress(PhaseFailed, st.Version, err.Error())
			return err
		}
	}
	var cur Installed
	if f.read(currentFile, &cur) && cur.Version == in.Version {
		if err := f.write(previousFile, cur); err != nil {
			return err
		}
	} else {
		f.remove(previousFile)
		cur = Installed{}
	}
	if err := f.write(currentFile, *st); err != nil {
		in.progress(PhaseFailed, st.Version, err.Error())
		return err
	}
	f.keepOnly(st.Version, cur.Version)
	// A new version clears the note about the last going back.
	f.setStatus(func(s *LaunchStatus) { s.Rollback, s.RollbackAt, s.Chosen, s.Left, s.To = "", nil, false, "", "" })
	in.progress(PhaseRestarting, st.Version, "")
	log.Printf("updates: switching from %s to %s", in.Version, st.Version)
	if in.Restart != nil {
		in.Restart()
	}
	return nil
}

// GoBack leaves the running downloaded version: it goes on the skip list
// and the launcher returns to the previous version (or the image's),
// putting the database copy back when one was made.
func (in *Installer) GoBack() error {
	if os.Getenv(envFromVolume) != "1" {
		return errors.New("this is the image's own version; there is nothing to go back to")
	}
	if !in.canGoBack() {
		return errNoCopy
	}
	f := in.folder()
	f.addSkip(in.Version)
	_ = f.write(goBackFile, Installed{Version: in.Version})
	in.progress(PhaseRestarting, in.Version, "")
	if in.Restart != nil {
		in.Restart()
	}
	return nil
}

// Previous is the version "Go back" returns to: a downloaded one, or the
// image's when none is kept ("" when the image itself runs).
func (in *Installer) Previous(imageVersion string) string {
	if os.Getenv(envFromVolume) != "1" {
		return ""
	}
	var p Installed
	if in.folder().read(previousFile, &p) && p.Version != "" {
		return p.Version
	}
	if !in.canGoBack() {
		return ""
	}
	if v := os.Getenv(envImageVersion); v != "" {
		return v
	}
	return imageVersion
}

// Progress is the install's current step.
func (in *Installer) Progress() Progress {
	in.mu.Lock()
	defer in.mu.Unlock()
	if in.prog.Phase == "" {
		return Progress{Phase: PhaseIdle}
	}
	return in.prog
}

// Launch is what the launcher last did.
func (in *Installer) Launch() LaunchStatus { return in.folder().status() }

func (in *Installer) waitIdle(ctx context.Context, version string) error {
	if in.Busy == nil {
		return nil
	}
	wait := in.IdleWait
	if wait <= 0 {
		wait = IdleWait
	}
	deadline := time.Now().Add(wait)
	for {
		what := in.Busy()
		if what == "" {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("still busy with " + what + "; try again when it finishes")
		}
		in.progress(PhaseWaiting, version, what)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(min(idlePoll, wait)):
		}
	}
}

// copyDB copies the databases. No database yet is fine: nothing to copy.
func (in *Installer) copyDB(f folder, version string) error {
	if in.Snapshot == nil {
		return nil
	}
	dir := f.path(dbCopyFolder)
	_ = os.RemoveAll(dir)
	tmp := dir + tmpSuffix
	_ = os.RemoveAll(tmp)
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		return err
	}
	if err := in.Snapshot(tmp); err != nil {
		_ = os.RemoveAll(tmp)
		return errors.New("the database could not be copied, so nothing was changed: " + err.Error())
	}
	if err := os.WriteFile(filepath.Join(tmp, copyForFile), []byte(version), 0o600); err != nil {
		_ = os.RemoveAll(tmp)
		return err
	}
	if err := os.Rename(tmp, dir); err != nil {
		_ = os.RemoveAll(tmp)
		return err
	}
	syncDir(filepath.Dir(dir))
	return nil
}

var errNoCopy = errors.New("this version changed the database and no copy from before it is kept, so going back is not safe")

// canGoBack is false when going back would reach the image's version on a
// database the running version changed, with no copy to put back.
func (in *Installer) canGoBack() bool {
	f := in.folder()
	var p, cur Installed
	if f.read(previousFile, &p) && p.Version != "" {
		return true
	}
	if !f.read(currentFile, &cur) || cur.Version != in.Version || !cur.Migrates {
		return true
	}
	return copyFor(f) == in.Version
}

func (in *Installer) progress(phase, version, msg string) {
	in.mu.Lock()
	defer in.mu.Unlock()
	in.prog = Progress{Phase: phase, Version: version, Message: msg, At: time.Now().UTC()}
}

func (in *Installer) folder() folder { return folder(filepath.Join(in.ConfigDir, AppFolder)) }

func (in *Installer) keys() []PublicKey {
	if in.Keys != nil {
		return in.Keys
	}
	return Pinned()
}
