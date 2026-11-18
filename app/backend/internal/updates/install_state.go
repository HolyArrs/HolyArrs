package updates

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// The installed-version files live in <config>/app. Each is replaced in one
// atomic rename, so a cut-off write leaves the old file whole.
const (
	AppFolder    = "app"
	currentFile  = "current.json"  // the version the launcher runs
	previousFile = "previous.json" // the one "Go back" returns to
	pendingFile  = "pending.json"  // a start not yet proven healthy
	skipFile     = "skip.json"     // versions never installed again
	statusFile   = "status.json"   // what the launcher last did
	keptListFile = "list.json"     // the last accepted signed update list
	dbCopyFolder = "dbcopy"        // the database copy made before a change
	copyForFile  = "for"           // inside dbCopyFolder: the version it was made for
	binaryName   = "holyarrs"
	tmpSuffix    = ".tmp"
)

// Env names the launcher and the app share.
const (
	// EnvMode is HOLYARRS_UPDATES=off|ask|auto; it overrides the setting.
	EnvMode = "HOLYARRS_UPDATES"
	// envFromVolume marks a program started by the launcher from the volume.
	envFromVolume = "HOLYARRS_FROM_VOLUME"
	// envImageBin is the image program, which a switch starts again.
	envImageBin = "HOLYARRS_IMAGE_BIN"
	// envImageVersion is the image program's version, which the
	// downloaded program cannot know by itself.
	envImageVersion = "HOLYARRS_IMAGE_VERSION"
)

// pendingTTL is how long an unproven start is remembered; after that the
// record no longer counts, so nothing stays stuck.
const pendingTTL = 24 * time.Hour

// Installed is one version on the volume.
type Installed struct {
	Version  string `json:"version"`
	Path     string `json:"path"`
	SHA256   string `json:"sha256"`
	Migrates bool   `json:"migrates,omitempty"`
}

// pending is a start the new version has not yet marked healthy.
type pending struct {
	Version  string    `json:"version"`
	Starts   int       `json:"starts"`
	Since    time.Time `json:"since"`
	Migrates bool      `json:"migrates,omitempty"`
}

// LaunchStatus is what the launcher last did, shown in Settings.
type LaunchStatus struct {
	// Source is "image" or "update": where the running program came from.
	Source string `json:"source"`
	// Rollback says which version was left and why.
	Rollback   string     `json:"rollback,omitempty"`
	RollbackAt *time.Time `json:"rollback_at,omitempty"`
	// ExecError is why a downloaded version could not be started.
	ExecError string `json:"exec_error,omitempty"`
	// Off says HOLYARRS_UPDATES=off made the image version run.
	Off bool `json:"off,omitempty"`
}

// folder is the installed-version folder of one config folder.
type folder string

func (f folder) path(name string) string { return filepath.Join(string(f), name) }

func (f folder) read(name string, v any) bool {
	b, err := os.ReadFile(f.path(name))
	return err == nil && json.Unmarshal(b, v) == nil
}

func (f folder) write(name string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return writeAtomic(f.path(name), b, 0o600)
}

func (f folder) remove(name string) { _ = os.Remove(f.path(name)) }

func (f folder) skip() []string {
	var s []string
	f.read(skipFile, &s)
	return s
}

func (f folder) addSkip(v string) {
	s := f.skip()
	if !slices.Contains(s, v) {
		_ = f.write(skipFile, append(s, v))
	}
}

func (f folder) status() LaunchStatus {
	var s LaunchStatus
	f.read(statusFile, &s)
	if s.Source == "" {
		s.Source = "image"
	}
	return s
}

func (f folder) setStatus(change func(*LaunchStatus)) {
	s := f.status()
	change(&s)
	_ = f.write(statusFile, s)
}

// clearTemps removes files left by a cut-off download or write.
func (f folder) clearTemps() {
	_ = filepath.WalkDir(string(f), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, tmpSuffix) {
			_ = os.Remove(p)
		}
		return nil
	})
}

// keepOnly removes version folders other than the named ones.
func (f folder) keepOnly(versions ...string) {
	es, _ := os.ReadDir(string(f))
	for _, e := range es {
		if e.IsDir() && e.Name() != dbCopyFolder && !slices.Contains(versions, e.Name()) {
			_ = os.RemoveAll(f.path(e.Name()))
		}
	}
}

// writeAtomic writes a temp file in the same folder, syncs it and renames
// it over p, so readers see the old file or the new one, never half.
func writeAtomic(p string, b []byte, mode os.FileMode) error {
	tmp := p + tmpSuffix
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, p); err != nil {
		os.Remove(tmp)
		return err
	}
	syncDir(filepath.Dir(p))
	return nil
}

func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		d.Close()
	}
}

// copyFor is the version the database copy was made for, or "" when there
// is no complete copy.
func copyFor(f folder) string {
	if _, err := os.Stat(filepath.Join(f.path(dbCopyFolder), database.MainFile)); err != nil {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(f.path(dbCopyFolder), copyForFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
