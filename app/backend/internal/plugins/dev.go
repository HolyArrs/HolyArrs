package plugins

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// Developer mode: the owner can run plugins that are not signed, from
// <plugins folder>/_dev/<id>/ (holyarrs-plugin.json and the built program,
// named plugin). They run in the same sandbox, ask for the same approval
// and are checked against the hash taken at install like any plugin; only
// the signed list is skipped. They never update on their own, and they
// start only while developer mode is on.

const (
	// devDirName starts with "_", which no plugin id can, so it never meets
	// an installed plugin's folder.
	devDirName      = "_dev"
	devManifestName = "holyarrs-plugin.json"
	devProgramName  = "plugin"
	devOffReason    = "developer mode is off"
)

// Developer mode refusals.
var (
	ErrDevOff     = errors.New("developer mode is off")
	ErrDevMissing = errors.New("no plugin with that id in the developer folder")
	ErrDevSigned  = errors.New("a signed plugin with that id is installed; remove it first")
)

// DevDir is the developer folder.
func (e *Engine) DevDir() string { return filepath.Join(e.Dir, devDirName) }

// devPlugin reads one plugin of the developer folder: its manifest as
// HolyArrs would publish it (with the program's hash), the manifest bytes
// and the program.
func (e *Engine) devPlugin(id string) (*Manifest, []byte, []byte, error) {
	if !idRe.MatchString(id) {
		return nil, nil, nil, ErrDevMissing
	}
	dir := filepath.Join(e.DevDir(), id)
	for _, p := range []string{e.DevDir(), dir, filepath.Join(dir, devManifestName), filepath.Join(dir, devProgramName)} {
		if fi, err := os.Lstat(p); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			return nil, nil, nil, fmt.Errorf("%w: %s is a link; copy the files in", ErrDevMissing, filepath.Base(p))
		}
	}
	src, err := os.ReadFile(filepath.Join(dir, devManifestName))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("%w: %s", ErrDevMissing, id)
	}
	prog, err := os.ReadFile(filepath.Join(dir, devProgramName))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("%w: %s has no %s file", ErrDevMissing, id, devProgramName)
	}
	var m map[string]any
	if err := json.Unmarshal(src, &m); err != nil {
		return nil, nil, nil, fmt.Errorf("%w: %s: %v", ErrManifest, devManifestName, err)
	}
	delete(m, "icon_file") // the release tools turn it into icon
	if m["id"] != id {
		return nil, nil, nil, fmt.Errorf("%w: %s names %v, its folder is %s", ErrManifest, devManifestName, m["id"], id)
	}
	sum := sha256.Sum256(prog)
	m["sha256"] = hex.EncodeToString(sum[:])
	raw, err := json.Marshal(m)
	if err != nil {
		return nil, nil, nil, err
	}
	man, err := ParseManifest(raw)
	if err != nil {
		return nil, nil, nil, err
	}
	return man, raw, prog, nil
}

// devIDs lists the plugin folders in the developer folder.
func (e *Engine) devIDs() []string {
	ents, err := os.ReadDir(e.DevDir())
	if err != nil {
		return nil
	}
	var ids []string
	for _, d := range ents {
		if d.IsDir() && idRe.MatchString(d.Name()) {
			ids = append(ids, d.Name())
		}
	}
	return ids
}

// InstallDev installs (or reloads) id from the developer folder and starts
// it. Developer mode must be on. Like any install it keeps an approval that
// still covers what it asks for and holds it when it asks for more.
func (e *Engine) InstallDev(ctx context.Context, id string, approve ...ApproveInput) (*Installed, error) {
	if err := e.init(); err != nil {
		return nil, err
	}
	if e.Runner == nil {
		return nil, ErrNotLinux
	}
	st, err := e.store.Load()
	if err != nil {
		return nil, err
	}
	if !st.DevMode {
		return nil, ErrDevOff
	}
	if p := st.Plugins[id]; p != nil && !p.Dev {
		return nil, ErrDevSigned // never swap a signed install for an unsigned one
	}
	m, raw, prog, err := e.devPlugin(id)
	if err != nil {
		return nil, err
	}
	if e.tooOld(m.MinAppVersion) {
		return nil, fmt.Errorf("%w: needs HolyArrs %s or newer", ErrNeedsNewerApp, m.MinAppVersion)
	}
	path, err := spoolFile(bytes.NewReader(prog), e.Dir, m.SHA256)
	if err != nil {
		return nil, err
	}
	acc := &Accepted{Manifest: m, FilePath: path, Dev: true}
	defer acc.Close()
	e.opMu.Lock()
	defer e.opMu.Unlock()
	if err := e.stopProc(id); err != nil {
		return nil, fmt.Errorf("stopping plugin: %w", err)
	}
	fresh, err := e.keepApproval(id)
	if err != nil {
		return nil, err
	}
	if err := e.store.Install(acc); err != nil {
		return nil, err
	}
	e.saveManifest(id, raw)
	if fresh {
		var ap *ApproveInput
		if len(approve) > 0 {
			ap = &approve[0]
		}
		if err := e.firstApproval(id, ap); err != nil {
			return nil, err
		}
	}
	in, err := e.installed(id)
	if err != nil {
		return nil, err
	}
	if !in.Enabled {
		if in.DisabledReason != devOffReason {
			return in, nil // stopped by the owner, or held for approval
		}
		if err := e.store.SetEnabled(id, true, ""); err != nil {
			return nil, err
		}
	}
	if err := e.start(id); err != nil {
		return nil, err
	}
	return e.installed(id)
}

// SetDevMode turns developer mode on or off. Off stops every developer
// plugin and keeps it off until developer mode is on and it is reloaded.
func (e *Engine) SetDevMode(on bool) error {
	if err := e.init(); err != nil {
		return err
	}
	e.opMu.Lock()
	defer e.opMu.Unlock()
	if err := e.store.update(func(st *State) { st.DevMode = on }); err != nil {
		return err
	}
	if on {
		return nil
	}
	st, err := e.store.Load()
	if err != nil {
		return err
	}
	var errs []error
	for id, p := range st.Plugins {
		if p.Dev {
			errs = append(errs, e.stopProc(id), e.store.SetEnabled(id, false, devOffReason))
		}
	}
	return errors.Join(errs...)
}

// SetTestVersions records whether id gets test versions; it takes effect
// at the next install or update.
func (e *Engine) SetTestVersions(id string, on bool) error {
	if err := e.init(); err != nil {
		return err
	}
	if !idRe.MatchString(id) {
		return fmt.Errorf("%w: %s", ErrNotListed, id)
	}
	return e.store.update(func(st *State) {
		if st.TestVersions == nil {
			st.TestVersions = map[string]bool{}
		}
		if on {
			st.TestVersions[id] = true
		} else {
			delete(st.TestVersions, id)
		}
	})
}

// TestEvent sends a running developer plugin one made-up event line, with
// "test": true in its payload, so its author can try their code. The event
// must be one the plugin asked for and the owner approved.
func (e *Engine) TestEvent(id, event string) error {
	if err := e.init(); err != nil {
		return err
	}
	in, err := e.installed(id)
	if err != nil {
		return err
	}
	if !in.Dev {
		return fmt.Errorf("%w: test events are only for developer plugins", ErrDevOff)
	}
	e.feedMu.Lock()
	defer e.feedMu.Unlock()
	f := e.feeds[id]
	if f == nil || f.closed || !slices.Contains(f.events, event) {
		return fmt.Errorf("%w: %s is not running or not approved for %s", ErrDevMissing, id, event)
	}
	line := eventLine(event, 0, map[string]any{"test": true, "at": time.Now().UTC(),
		"movie": map[string]any{"id": 0, "name": "Test Movie", "year": 2000, "tmdb": "0", "imdb": "tt0000000"}})
	if line == nil {
		return errors.New("the test event could not be written")
	}
	select {
	case f.q <- feedItem{line: line}:
		return nil
	default:
		return errors.New("the plugin's event queue is full")
	}
}

// verifyDevStart checks a developer plugin at start: developer mode is on
// and the program is still the one installed.
func (s *Store) verifyDevStart(st *State, p *Installed) ([]byte, error) {
	if !st.DevMode {
		return nil, ErrDevOff
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
	return file, nil
}

// update changes the state under the store lock.
func (s *Store) update(change func(*State)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.dir, 0o700); err != nil { // nothing installed yet
		return err
	}
	st, err := s.Load()
	if err != nil {
		return err
	}
	change(st)
	return s.save(st)
}

// showTestVersions marks the rows that have a newer test version, and
// offers it (in newest) where the owner asked for test versions.
func (e *Engine) showTestVersions(ctx context.Context, st *State, rows map[string]*Plugin, newest map[string]Entry) {
	raw := e.testList(ctx, st, false)
	if raw == nil {
		return
	}
	tl, _, err := e.verify(raw, KindTest, 0)
	if err != nil {
		return
	}
	for _, en := range tl.Entries {
		p, ok := rows[en.ID]
		if !ok || !newerVersion(en.Version, p.Version) || (p.TestVersion != "" && !newerVersion(en.Version, p.TestVersion)) {
			continue
		}
		p.TestVersion = en.Version
		if st.TestVersions[en.ID] {
			newest[en.ID] = en
		}
	}
}

// showDev adds the developer folder's plugins to the rows while developer
// mode is on, and returns order with any new ids.
func (e *Engine) showDev(st *State, rows map[string]*Plugin, order []string) []string {
	if !st.DevMode {
		return order
	}
	for _, id := range e.devIDs() {
		p, ok := rows[id]
		if !ok {
			p = &Plugin{ID: id}
			rows[id] = p
			order = append(order, id)
		}
		if p.Installed && !p.Dev {
			continue // a signed install of the same id stays as it is
		}
		p.Dev = true
		m, _, _, err := e.devPlugin(id)
		if err != nil {
			p.DevError = err.Error()
			continue
		}
		p.Version = m.Version
		if !p.Installed {
			p.show(m)
		}
	}
	return order
}
