package plugins

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// Store refusals.
var (
	ErrStateCorrupt = errors.New("plugin state file is corrupt")
	ErrNotInstalled = errors.New("plugin is not installed")
	ErrDisabled     = errors.New("plugin is disabled")
	ErrCounterOlder = errors.New("accepted list is older than the stored one")
)

// RemoveMode says what Remove does with a plugin's files.
type RemoveMode string

const (
	// RemoveDelete wipes the plugin's folder and its state entry.
	RemoveDelete RemoveMode = "delete"
)

// keptReason marks a plugin removed by an earlier build that kept its data.
// The engine wipes such leftovers at start, so they show as not installed.
const keptReason = "removed, data kept"

const (
	stateName   = "state.json"
	programName = "program"
	dataName    = "data"
)

// State is what the Engine remembers between starts.
type State struct {
	ListCounter  uint64 `json:"list_counter"`
	BlockCounter uint64 `json:"block_counter"`
	// TestCounter is the counter of the last test-version list accepted.
	TestCounter uint64 `json:"test_counter,omitempty"`
	// DevMode is the owner's choice to run unsigned plugins from the
	// developer folder (dev.go); off by default.
	DevMode bool `json:"dev_mode,omitempty"`
	// TestVersions lists the plugins whose owner asked for test versions.
	TestVersions map[string]bool `json:"test_versions,omitempty"`
	// BlockCheckedAt is when the blocklist last verified.
	BlockCheckedAt time.Time `json:"block_checked_at,omitempty"`
	// AllowNoFileLock is the owner's one choice, for the whole install, to
	// let signed plugins start when the system cannot give them the file
	// lock; memory, process and CPU limits stay on. NoFileLockAsked is true
	// once the owner answered.
	AllowNoFileLock bool `json:"allow_no_file_lock,omitempty"`
	NoFileLockAsked bool `json:"no_file_lock_asked,omitempty"`
	// AllowNoNetLock is the same choice for the network lock: plugins that
	// list hosts may start when the system cannot keep them to those hosts.
	// UDP, raw sockets and the proxy settings stay on.
	AllowNoNetLock bool                  `json:"allow_no_net_lock,omitempty"`
	NoNetLockAsked bool                  `json:"no_net_lock_asked,omitempty"`
	Plugins        map[string]*Installed `json:"plugins"`
}

// Installed is one installed plugin.
type Installed struct {
	ID             string `json:"id"`
	Version        string `json:"version"`
	SHA256         string `json:"sha256"`
	Dir            string `json:"dir"`
	Enabled        bool   `json:"enabled"`
	DisabledReason string `json:"disabled_reason,omitempty"`
	// AllowNoFileLock is the old per-plugin choice; Load folds it into the
	// install-wide State.AllowNoFileLock and it is no longer written.
	AllowNoFileLock bool `json:"allow_no_file_lock,omitempty"`
	// Approval is what the owner approved: hosts, folders and the real
	// folders chosen. Installs from before 1.2.1 have none (see approvalFor).
	Approval *Approval `json:"approval,omitempty"`
	// Pending is an update waiting for the owner's approval; this version
	// keeps running meanwhile.
	Pending *Pending `json:"pending,omitempty"`
	// Key is the signing key (hex) of the list this version came from; an
	// update signed by another key is refused.
	Key string `json:"key,omitempty"`
	// Dev is an unsigned plugin from the developer folder; it starts only
	// while developer mode is on. Test is a version from the test list.
	Dev  bool `json:"dev,omitempty"`
	Test bool `json:"test,omitempty"`
}

// Store keeps plugin programs, data folders and state under one folder.
// mu is held from Load to save so no update is lost.
type Store struct {
	mu  sync.Mutex
	dir string
}

// writeProgram and writeState write files; vars so tests can break an
// install half way.
var (
	writeProgram = writeReaderSynced
	writeState   = writeFileSynced
)

// versionDir names a version's folder by its content, so a reinstall of the
// same version with other bytes never overwrites the program in use.
func versionDir(version, sha string) string {
	return version + "-" + sha[:16]
}

// NewStore uses dir as the plugins folder. The folder is made by the first
// install; until then a missing folder reads as no plugins.
func NewStore(dir string) (*Store, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("plugins folder: %w", err)
	}
	return &Store{dir: dir}, nil
}

// Load reads the state. A missing file is an empty state; a file that does
// not parse is refused and left as it is, so nothing is silently forgotten.
func (s *Store) Load() (*State, error) {
	data, err := os.ReadFile(filepath.Join(s.dir, stateName))
	if errors.Is(err, os.ErrNotExist) {
		return &State{Plugins: map[string]*Installed{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading plugin state: %w", err)
	}
	var st State
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&st); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStateCorrupt, err)
	}
	if err := requireEOF(dec); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStateCorrupt, err)
	}
	if st.Plugins == nil {
		st.Plugins = map[string]*Installed{}
	}
	for _, p := range st.Plugins {
		if p != nil && p.AllowNoFileLock {
			st.AllowNoFileLock, st.NoFileLockAsked, p.AllowNoFileLock = true, true, false
		}
	}
	for id, p := range st.Plugins {
		if p == nil || p.ID != id || !idRe.MatchString(id) || !semverRe.MatchString(p.Version) || !validSHA256(p.SHA256) ||
			p.Dir != versionDir(p.Version, p.SHA256) {
			return nil, fmt.Errorf("%w: bad entry %q", ErrStateCorrupt, id)
		}
	}
	return &st, nil
}

// Install writes the checked bytes as the plugin's program and records it.
// The old version stays on disk and in state until the new one is fully
// written, so a failed install leaves the old version working.
func (s *Store) Install(a *Accepted) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.Load()
	if err != nil {
		return err
	}
	last := st.ListCounter
	if a.Kind == KindTest {
		last = st.TestCounter
	}
	if !a.Dev && (a.ListCounter < last || a.BlockCounter < st.BlockCounter) {
		return ErrCounterOlder
	}
	m := a.Manifest
	verName := versionDir(m.Version, m.SHA256)
	verDir := filepath.Join(s.dir, m.ID, verName)
	if err := os.MkdirAll(verDir, 0o700); err != nil {
		return fmt.Errorf("plugin folder: %w", err)
	}
	syncDir(s.dir)
	syncDir(filepath.Dir(verDir))
	if err := s.checkDirs(m.ID, verName); err != nil {
		return err
	}
	tmp := filepath.Join(verDir, programName+".new")
	src, err := os.Open(a.FilePath)
	if err != nil {
		return fmt.Errorf("checked plugin file: %w", err)
	}
	h := sha256.New()
	err = writeProgram(tmp, io.TeeReader(src, h))
	src.Close()
	if err != nil {
		os.Remove(tmp)
		return fmt.Errorf("writing plugin program: %w", err)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != m.SHA256 {
		os.Remove(tmp)
		return fmt.Errorf("%w: checked file changed before install", ErrTampered)
	}
	if err := os.Rename(tmp, filepath.Join(verDir, programName)); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("placing plugin program: %w", err)
	}
	syncDir(verDir)

	old := st.Plugins[m.ID]
	switch {
	case a.Dev:
	case a.Kind == KindTest:
		st.TestCounter, st.BlockCounter, st.BlockCheckedAt = a.ListCounter, a.BlockCounter, time.Now().UTC()
	default:
		st.ListCounter, st.BlockCounter, st.BlockCheckedAt = a.ListCounter, a.BlockCounter, time.Now().UTC()
	}
	in := &Installed{ID: m.ID, Version: m.Version, SHA256: m.SHA256, Dir: verName, Enabled: true, Dev: a.Dev, Test: a.Kind == KindTest}
	if old != nil {
		in.Approval = old.Approval // an update that asks for more is held at start until approved
		in.Key = old.Key
	}
	// An update keeps a plugin the owner stopped stopped; one turned off by a
	// check or a failed start gets its fresh chance.
	if old != nil && !old.Enabled && ownerStopped(old.DisabledReason) {
		in.Enabled, in.DisabledReason = false, old.DisabledReason
	}
	st.Plugins[m.ID] = in
	if err := s.save(st); err != nil {
		return err
	}
	if old != nil && old.Dir != verName {
		os.RemoveAll(filepath.Join(s.dir, m.ID, old.Dir))
	}
	return nil
}

// Remove deletes a plugin with its program, settings and data.
// stop, when given, ends the running program first.
func (s *Store) Remove(id string, mode RemoveMode, stop ...func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.Load()
	if err != nil {
		return err
	}
	if _, ok := st.Plugins[id]; !ok {
		return fmt.Errorf("%w: %s", ErrNotInstalled, id)
	}
	if mode != RemoveDelete {
		return fmt.Errorf("unknown remove mode %q", mode)
	}
	for _, f := range stop {
		if f == nil {
			continue
		}
		if err := f(); err != nil {
			return fmt.Errorf("stopping plugin: %w", err)
		}
	}
	// Move the folder aside first, then forget it, then wipe it.
	dir := filepath.Join(s.dir, id)
	trash := filepath.Join(s.dir, ".trash-"+id+"-"+randomName())
	if err := os.Rename(dir, trash); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing plugin folder: %w", err)
	}
	syncDir(s.dir)
	delete(st.Plugins, id)
	if err := s.save(st); err != nil {
		os.Rename(trash, dir)
		return err
	}
	return os.RemoveAll(trash)
}

// ProgramPath is where an installed plugin's program lives.
func (s *Store) ProgramPath(p *Installed) string {
	return filepath.Join(s.dir, p.ID, p.Dir, programName)
}

// checkDirs refuses a plugin or version folder that is not a plain folder,
// such as a symlink pointing elsewhere.
func (s *Store) checkDirs(id, verName string) error {
	for _, dir := range []string{filepath.Join(s.dir, id), filepath.Join(s.dir, id, verName)} {
		fi, err := os.Lstat(dir)
		if err != nil {
			return fmt.Errorf("plugin folder: %w", err)
		}
		if !fi.Mode().IsDir() {
			return fmt.Errorf("plugin folder %s is not a plain folder", dir)
		}
	}
	return nil
}

// DataDir is the plugin's own data folder, the only folder it is given.
func (s *Store) DataDir(id string) string {
	return filepath.Join(s.dir, id, dataName)
}

// save writes state to a temp file and renames it over the old one.
func (s *Store) save(st *State) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(s.dir, stateName)
	tmp := path + ".new"
	if err := writeState(tmp, data); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("writing plugin state: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("saving plugin state: %w", err)
	}
	syncDir(s.dir)
	return nil
}

// syncDir flushes a folder's entries after a rename or create. Windows
// cannot sync folders, so its error is ignored there.
func syncDir(dir string) error {
	f, err := os.Open(dir)
	if err == nil {
		err = f.Sync()
		f.Close()
	}
	if runtime.GOOS == "windows" {
		return nil
	}
	return err
}

func randomName() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// SaveCounters records the counters of newly verified lists and the check
// time. A counter below the stored one is refused, so a slower check can
// never roll the stored counters back.
func (s *Store) SaveCounters(list, block uint64, checked time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.Load()
	if err != nil {
		return err
	}
	if list < st.ListCounter || block < st.BlockCounter {
		return ErrCounterOlder
	}
	st.ListCounter, st.BlockCounter, st.BlockCheckedAt = list, block, checked.UTC()
	return s.save(st)
}

func writeReaderSynced(path string, r io.Reader) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o700)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Chmod(path, 0o700)
}

func writeFileSynced(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o700)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Chmod(path, 0o700)
}

// SetEnabled turns a plugin on or off with the reason shown while off.
func (s *Store) SetEnabled(id string, on bool, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.Load()
	if err != nil {
		return err
	}
	p, ok := st.Plugins[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrNotInstalled, id)
	}
	if on {
		reason = ""
	}
	p.Enabled, p.DisabledReason = on, reason
	return s.save(st)
}

// SetApproval stores what the owner approved for id.
func (s *Store) SetApproval(id string, a *Approval) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.Load()
	if err != nil {
		return err
	}
	p, ok := st.Plugins[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrNotInstalled, id)
	}
	p.Approval = a
	return s.save(st)
}

// SetAllowNoNetLock records the owner's install-wide choice to let plugins
// with hosts start without the network lock.
func (s *Store) SetAllowNoNetLock(on bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.Load()
	if err != nil {
		return err
	}
	st.AllowNoNetLock, st.NoNetLockAsked = on, true
	return s.save(st)
}

// SetAllowNoFileLock records the owner's install-wide choice to let signed
// plugins start without the file lock.
func (s *Store) SetAllowNoFileLock(on bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.Load()
	if err != nil {
		return err
	}
	st.AllowNoFileLock, st.NoFileLockAsked = on, true
	return s.save(st)
}
