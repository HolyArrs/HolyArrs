package plugins

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// ErrPublisherChanged refuses an update signed by another key than the
// installed version was.
var ErrPublisherChanged = errors.New("this update is signed by another key than the installed version; remove and reinstall the plugin to trust it")

// Pending is an update that asks for more than the owner approved. It waits
// beside the installed version, which keeps running, until the owner
// approves it.
type Pending struct {
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
	// Test is a version from the test list.
	Test bool `json:"test,omitempty"`
}

const pendingName = "pending"

func (s *Store) pendingDir(id string) string { return filepath.Join(s.dir, id, pendingName) }

// SetPending records (or with nil clears) id's waiting update.
func (s *Store) SetPending(id string, p *Pending) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.Load()
	if err != nil {
		return err
	}
	in, ok := st.Plugins[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrNotInstalled, id)
	}
	in.Pending = p
	return s.save(st)
}

// SetKey records the key that signed id's installed version.
func (s *Store) SetKey(id, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.Load()
	if err != nil {
		return err
	}
	if in, ok := st.Plugins[id]; ok {
		in.Key = key
		return s.save(st)
	}
	return nil
}

func keyHex(k ed25519.PublicKey) string { return hex.EncodeToString(k) }

// ErrPublisherUnknown refuses an update while the installed version's
// signing key is not known yet.
var ErrPublisherUnknown = errors.New("the installed version's signing key is not known yet; start the plugin once, then update it")

// checkPublisher refuses an update of id signed by another key. An install
// from before keys were kept learns its key from the cached signed list
// first; when even that can't tell, the update waits.
func (e *Engine) checkPublisher(id string, k ed25519.PublicKey) error {
	old, _ := e.installed(id)
	if old == nil {
		return nil
	}
	if old.Key == "" {
		if in, err := e.startInput(); err == nil {
			e.backfillKey(id, in.PublicKey)
		}
		if old, _ = e.installed(id); old == nil || old.Key == "" {
			return ErrPublisherUnknown
		}
	}
	if old.Key != keyHex(k) {
		return ErrPublisherChanged
	}
	return nil
}

// backfillKey records the key that signs the cached list as id's key, for an
// install from before keys were kept, when that list holds the installed
// version.
func (e *Engine) backfillKey(id string, k ed25519.PublicKey) {
	old, _ := e.installed(id)
	if old == nil || old.Key != "" || len(k) == 0 {
		return
	}
	raw, err := os.ReadFile(filepath.Join(e.Dir, e.cacheListName()))
	if err != nil {
		return
	}
	l, err := VerifyList(k, raw, KindPlugins, 0)
	if err != nil {
		return
	}
	for _, en := range l.Entries {
		if en.ID == id && en.Version == old.Version && en.SHA256 == old.SHA256 {
			_ = e.store.SetKey(id, keyHex(k))
			return
		}
	}
}

// holdUpdate keeps an update that asks for more than the owner approved
// beside the running version, which keeps running. It reports whether the
// update was held (and answers ErrNeedsApproval then).
func (e *Engine) holdUpdate(id string, acc *Accepted, manRaw []byte) (bool, error) {
	old, _ := e.installed(id)
	if old == nil || old.Approval == nil || old.Version == acc.Manifest.Version && old.SHA256 == acc.Manifest.SHA256 {
		return false, nil
	}
	cur := e.readManifest(id)
	if cur == nil || cur.Version != old.Version || len(accessDiff(cur, *old.Approval)) > 0 {
		return false, nil // the running version is not approved either: install as before
	}
	if len(accessDiff(acc.Manifest, approvalFor(old, acc.Manifest))) == 0 {
		return false, nil
	}
	dir := e.store.pendingDir(id)
	if err := os.RemoveAll(dir); err != nil {
		return true, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return true, err
	}
	src, err := os.Open(acc.FilePath)
	if err != nil {
		return true, err
	}
	err = writeProgram(filepath.Join(dir, programName), src)
	src.Close()
	if err != nil {
		return true, err
	}
	if err := writeFileSynced(filepath.Join(dir, "manifest.json"), manRaw); err != nil {
		return true, err
	}
	if err := e.store.SetPending(id, &Pending{Version: acc.Manifest.Version, SHA256: acc.Manifest.SHA256, Test: acc.Kind == KindTest}); err != nil {
		return true, err
	}
	return true, ErrNeedsApproval
}

// pendingManifest is id's waiting update's manifest, or nil.
func (e *Engine) pendingManifest(id string, in *Installed) *Manifest {
	if in == nil || in.Pending == nil {
		return nil
	}
	raw, err := os.ReadFile(filepath.Join(e.store.pendingDir(id), "manifest.json"))
	if err != nil {
		return nil
	}
	m, err := ParseManifest(raw)
	if err != nil || m.ID != id || m.Version != in.Pending.Version || m.SHA256 != in.Pending.SHA256 {
		return nil
	}
	return m
}

// promotePending makes id's waiting update the installed version: its
// program is checked against the hash the signed list gave and installed
// the usual way, then the old version's folder goes. Called with opMu held,
// right before the owner's approval is stored.
func (e *Engine) promotePending(id string) error {
	in, err := e.installed(id)
	if err != nil || in.Pending == nil {
		return err
	}
	m := e.pendingManifest(id, in)
	if m == nil {
		_ = e.store.SetPending(id, nil)
		return fmt.Errorf("%w: the waiting update is unreadable; install it again", ErrNotInstalled)
	}
	st, err := e.store.Load()
	if err != nil {
		return err
	}
	_ = e.stopProc(id)
	dir := e.store.pendingDir(id)
	acc := &Accepted{Manifest: m, FilePath: filepath.Join(dir, programName), ListCounter: st.ListCounter, BlockCounter: st.BlockCounter}
	if in.Pending.Test {
		acc.Kind, acc.ListCounter = KindTest, st.TestCounter
	}
	if err := e.store.Install(acc); err != nil {
		return err
	}
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return err
	}
	e.saveManifest(id, raw)
	if err := e.store.SetPending(id, nil); err != nil {
		return err
	}
	return os.RemoveAll(dir)
}

// UnmarshalJSON reads an approval; empty owner limits ({} or null, all
// boxes left blank) mean no limits of the owner's own.
func (in *ApproveInput) UnmarshalJSON(b []byte) error {
	var raw struct {
		Folders    map[string]string `json:"folders"`
		OwnerHosts *[]string         `json:"owner_hosts,omitempty"`
		Limits     json.RawMessage   `json:"limits,omitempty"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	in.Folders, in.OwnerHosts, in.Limits = raw.Folders, raw.OwnerHosts, nil
	if l := strings.TrimSpace(string(raw.Limits)); l != "" && l != "null" && strings.ReplaceAll(l, " ", "") != "{}" {
		var r Resources
		if err := json.Unmarshal(raw.Limits, &r); err != nil {
			return err
		}
		in.Limits = &r
	}
	return nil
}

// keepForwarding is the 1.3.0 upgrade step: Tailscale, the one plugin that
// forwarded before scopes existed, keeps forwarding when its installed
// version's manifest doesn't ask for network.forward yet.
func (e *Engine) keepForwarding() {
	in, err := e.installed("tailscale")
	if err != nil || in.Approval == nil {
		return
	}
	m := e.readManifest("tailscale")
	if m == nil || slices.Contains(m.Access.Actions, "network.forward") || slices.Contains(in.Approval.Kept, "network.forward") {
		return
	}
	a := *in.Approval
	a.Kept = append(slices.Clone(a.Kept), "network.forward")
	_ = e.store.SetApproval("tailscale", &a)
}
