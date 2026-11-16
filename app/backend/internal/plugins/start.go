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
	"path/filepath"
	"strings"
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
	s.mu.Lock()
	defer s.mu.Unlock()
	p, _, err := s.checkStart(id, in)
	return p, err
}

// checkStart is CheckStart with s.mu held; it also returns the checked bytes.
func (s *Store) checkStart(id string, in StartInput) (*Installed, []byte, error) {
	st, err := s.Load()
	if err != nil {
		return nil, nil, err
	}
	p, ok := st.Plugins[id]
	if !ok {
		return nil, nil, fmt.Errorf("%w: %s", ErrNotInstalled, id)
	}
	if !p.Enabled {
		return nil, nil, fmt.Errorf("%w: %s", ErrDisabled, p.DisabledReason)
	}
	file, err := s.verifyStart(st, p, in)
	if err != nil {
		p.Enabled, p.DisabledReason = false, err.Error()
		if serr := s.save(st); serr != nil {
			return nil, nil, errors.Join(err, serr)
		}
		return nil, nil, err
	}
	return p, file, s.save(st)
}

// Start checks the plugin and starts it with only its own data folder. The
// checked bytes are copied to a fresh run file, which is hashed again and
// run, so nothing can swap the program between the check and the start.
// Of env only HOLYARRS_PLUGIN_* keys are passed; PATH, TZ and HOME (the
// data folder) are added here.
func (s *Store) Start(ctx context.Context, r Runner, id string, in StartInput, args, env []string) (Process, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, file, err := s.checkStart(id, in)
	if err != nil {
		return nil, err
	}
	data := s.DataDir(id)
	if err := os.MkdirAll(data, 0o700); err != nil {
		return nil, fmt.Errorf("plugin data folder: %w", err)
	}
	syncDir(filepath.Dir(data))
	verDir := filepath.Dir(s.ProgramPath(p))
	run := filepath.Join(verDir, "run-"+randomName())
	if err := writeFileSynced(run, file); err != nil {
		os.Remove(run)
		return nil, fmt.Errorf("plugin run copy: %w", err)
	}
	syncDir(verDir)
	if got, err := hashFile(run); err != nil || got != p.SHA256 {
		os.Remove(run)
		return nil, fmt.Errorf("%w: run copy %s", ErrTampered, got)
	}
	proc, err := r.Start(ctx, Spec{ID: id, Program: run, Args: args, Env: pluginEnv(env, data), DataDir: data})
	if err != nil {
		os.Remove(run)
		return nil, err
	}
	old, _ := filepath.Glob(filepath.Join(verDir, "run-*"))
	for _, f := range old {
		if f != run {
			os.Remove(f)
		}
	}
	return proc, nil
}

// pluginEnv keeps only HOLYARRS_PLUGIN_* keys from env and adds PATH, TZ
// and HOME.
func pluginEnv(env []string, home string) []string {
	out := []string{"HOME=" + home}
	for _, k := range []string{"PATH", "TZ"} {
		if v, ok := os.LookupEnv(k); ok {
			out = append(out, k+"="+v)
		}
	}
	for _, kv := range env {
		if strings.HasPrefix(kv, "HOLYARRS_PLUGIN_") {
			out = append(out, kv)
		}
	}
	return out
}

// verifyStart also moves the stored counters forward when newer lists pass,
// so an older list is never accepted again. It returns the checked bytes.
func (s *Store) verifyStart(st *State, p *Installed, in StartInput) ([]byte, error) {
	list, err := VerifyList(in.PublicKey, in.List, KindPlugins, st.ListCounter)
	if err != nil {
		return nil, fmt.Errorf("plugin list: %w", err)
	}
	block, err := VerifyList(in.PublicKey, in.Blocklist, KindBlocklist, st.BlockCounter)
	if err != nil {
		return nil, fmt.Errorf("blocklist: %w", err)
	}
	if block.Counter < *list.MinBlocklistCounter {
		return nil, fmt.Errorf("%w: counter %d, need %d", ErrStaleBlock, block.Counter, *list.MinBlocklistCounter)
	}
	if err := s.checkDirs(p.ID, p.Dir); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTampered, err)
	}
	file, err := os.ReadFile(s.ProgramPath(p))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTampered, err)
	}
	sum := sha256.Sum256(file)
	if got := hex.EncodeToString(sum[:]); got != p.SHA256 {
		return nil, fmt.Errorf("%w: file %s, installed %s", ErrTampered, got, p.SHA256)
	}
	m := &Manifest{ID: p.ID, Version: p.Version, SHA256: p.SHA256}
	for _, e := range block.Entries {
		if e.blocks(m) {
			return nil, fmt.Errorf("%w: %s %s", ErrBlocked, p.ID, p.Version)
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
		return nil, fmt.Errorf("%w: %s %s", ErrNotOnList, p.ID, p.Version)
	}
	st.ListCounter, st.BlockCounter = list.Counter, block.Counter
	return file, nil
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
