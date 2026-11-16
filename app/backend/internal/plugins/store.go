package plugins

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	// RemoveKeep disables the plugin and keeps its program and data.
	RemoveKeep RemoveMode = "keep"
)

const (
	stateName   = "state.json"
	programName = "program"
	dataName    = "data"
)

// State is what the Engine remembers between starts.
type State struct {
	ListCounter  uint64                `json:"list_counter"`
	BlockCounter uint64                `json:"block_counter"`
	Plugins      map[string]*Installed `json:"plugins"`
}

// Installed is one installed plugin.
type Installed struct {
	ID             string `json:"id"`
	Version        string `json:"version"`
	SHA256         string `json:"sha256"`
	Enabled        bool   `json:"enabled"`
	DisabledReason string `json:"disabled_reason,omitempty"`
}

// Store keeps plugin programs, data folders and state under one folder.
type Store struct {
	dir string
}

// writeProgram writes a program file; a var so tests can break an install
// half way.
var writeProgram = writeFileSynced

// NewStore uses dir as the plugins folder, creating it when missing.
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
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
	for id, p := range st.Plugins {
		if p == nil || p.ID != id || !idRe.MatchString(id) || !semverRe.MatchString(p.Version) || !validSHA256(p.SHA256) {
			return nil, fmt.Errorf("%w: bad entry %q", ErrStateCorrupt, id)
		}
	}
	return &st, nil
}

// Install writes the checked bytes as the plugin's program and records it.
// The old version stays on disk and in state until the new one is fully
// written, so a failed install leaves the old version working.
func (s *Store) Install(a *Accepted) error {
	st, err := s.Load()
	if err != nil {
		return err
	}
	if a.ListCounter < st.ListCounter || a.BlockCounter < st.BlockCounter {
		return ErrCounterOlder
	}
	m := a.Manifest
	verDir := filepath.Join(s.dir, m.ID, m.Version)
	if err := os.MkdirAll(verDir, 0o700); err != nil {
		return fmt.Errorf("plugin folder: %w", err)
	}
	tmp := filepath.Join(verDir, programName+".new")
	if err := writeProgram(tmp, a.File); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("writing plugin program: %w", err)
	}
	if err := os.Rename(tmp, filepath.Join(verDir, programName)); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("placing plugin program: %w", err)
	}

	old := st.Plugins[m.ID]
	st.ListCounter, st.BlockCounter = a.ListCounter, a.BlockCounter
	st.Plugins[m.ID] = &Installed{ID: m.ID, Version: m.Version, SHA256: m.SHA256, Enabled: true}
	if err := s.save(st); err != nil {
		return err
	}
	if old != nil && old.Version != m.Version {
		os.RemoveAll(filepath.Join(s.dir, m.ID, old.Version))
	}
	return nil
}

// Remove deletes a plugin with its data, or disables it and keeps the data.
func (s *Store) Remove(id string, mode RemoveMode) error {
	st, err := s.Load()
	if err != nil {
		return err
	}
	p, ok := st.Plugins[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrNotInstalled, id)
	}
	switch mode {
	case RemoveDelete:
		delete(st.Plugins, id)
		if err := s.save(st); err != nil {
			return err
		}
		return os.RemoveAll(filepath.Join(s.dir, id))
	case RemoveKeep:
		p.Enabled, p.DisabledReason = false, "removed, data kept"
		return s.save(st)
	}
	return fmt.Errorf("unknown remove mode %q", mode)
}

// ProgramPath is where a plugin version's program lives.
func (s *Store) ProgramPath(id, version string) string {
	return filepath.Join(s.dir, id, version, programName)
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
	if err := writeFileSynced(tmp, data); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("writing plugin state: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("saving plugin state: %w", err)
	}
	return nil
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
