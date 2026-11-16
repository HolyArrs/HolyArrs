package plugins

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
)

// ErrTampered marks a program file on disk that no longer matches state.
var ErrTampered = errors.New("plugin program on disk does not match the installed one")

// StartInput holds the current signed lists, checked against the counters
// in state.
type StartInput struct {
	PublicKey ed25519.PublicKey
	List      []byte
	Blocklist []byte
}

// CheckStart re-hashes the program on disk and checks it against state and
// the current signed lists. Any mismatch disables the plugin with the reason.
func (s *Store) CheckStart(id string, in StartInput) (*Installed, error) {
	st, err := s.Load()
	if err != nil {
		return nil, err
	}
	p, ok := st.Plugins[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotInstalled, id)
	}
	if !p.Enabled {
		return nil, fmt.Errorf("%w: %s", ErrDisabled, p.DisabledReason)
	}
	if err := s.verifyStart(st, p, in); err != nil {
		p.Enabled, p.DisabledReason = false, err.Error()
		if serr := s.save(st); serr != nil {
			return nil, errors.Join(err, serr)
		}
		return nil, err
	}
	return p, s.save(st)
}

// Start checks the plugin and starts it with only its own data folder.
func (s *Store) Start(ctx context.Context, r Runner, id string, in StartInput, args, env []string) (Process, error) {
	p, err := s.CheckStart(id, in)
	if err != nil {
		return nil, err
	}
	data := s.DataDir(id)
	if err := os.MkdirAll(data, 0o700); err != nil {
		return nil, fmt.Errorf("plugin data folder: %w", err)
	}
	return r.Start(ctx, Spec{Program: s.ProgramPath(id, p.Version), Args: args, Env: env, DataDir: data})
}

// verifyStart also moves the stored counters forward when newer lists pass,
// so an older list is never accepted again.
func (s *Store) verifyStart(st *State, p *Installed, in StartInput) error {
	list, err := VerifyList(in.PublicKey, in.List, KindPlugins, st.ListCounter)
	if err != nil {
		return fmt.Errorf("plugin list: %w", err)
	}
	block, err := VerifyList(in.PublicKey, in.Blocklist, KindBlocklist, st.BlockCounter)
	if err != nil {
		return fmt.Errorf("blocklist: %w", err)
	}
	if block.Counter < *list.MinBlocklistCounter {
		return fmt.Errorf("%w: counter %d, need %d", ErrStaleBlock, block.Counter, *list.MinBlocklistCounter)
	}
	got, err := hashFile(s.ProgramPath(p.ID, p.Version))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrTampered, err)
	}
	if got != p.SHA256 {
		return fmt.Errorf("%w: file %s, installed %s", ErrTampered, got, p.SHA256)
	}
	m := &Manifest{ID: p.ID, Version: p.Version, SHA256: p.SHA256}
	for _, e := range block.Entries {
		if e.blocks(m) {
			return fmt.Errorf("%w: %s %s", ErrBlocked, p.ID, p.Version)
		}
	}
	onList := false
	for _, e := range list.Entries {
		if e.ID == p.ID && e.Version == p.Version && e.SHA256 == p.SHA256 {
			onList = true
			break
		}
	}
	if !onList {
		return fmt.Errorf("%w: %s %s", ErrNotOnList, p.ID, p.Version)
	}
	st.ListCounter, st.BlockCounter = list.Counter, block.Counter
	return nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
