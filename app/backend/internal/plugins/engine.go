package plugins

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Engine refusals.
var (
	ErrNoKey     = errors.New("plugin signing key not set yet")
	ErrNotLinux  = errors.New("plugins run only on Linux")
	ErrNotListed = errors.New("plugin is not on the plugin list")
	ErrFetch     = errors.New("could not fetch from the plugin list server")
)

// Cache and manifest names in the plugins folder. They hold a dot, which a
// plugin id never does, so they never meet a plugin's own folder.
const (
	cacheList      = "list.json"
	cacheBlocklist = "blocklist.json"
	manifestDir    = ".manifests"
)

// maxListSize caps a fetched list or manifest.
const maxListSize = 4 << 20

// DefaultBlockEvery is how often the blocklist is checked.
const DefaultBlockEvery = 6 * time.Hour

// Engine installs, starts and stops plugins. Nothing runs before the first
// install: with no plugin installed, start-up and the blocklist check do
// nothing.
type Engine struct {
	// Dir is the plugins folder.
	Dir string
	// ListURL is the https base the lists and plugin files are served from:
	// plugins.json, blocklist.json and plugins/{id}/{version}/manifest.json
	// and .../plugin below it.
	ListURL string
	// Keys are the pinned public keys; empty refuses every install.
	Keys []ed25519.PublicKey
	// Runner starts plugins; nil means plugins cannot run here (not Linux).
	Runner Runner
	// Limits are the caps the sandbox applies, shown on the list.
	Limits ActiveLimits
	// HTTP fetches lists and files; nil uses a client with a timeout.
	HTTP *http.Client
	// BlockEvery is the blocklist check interval; 0 means DefaultBlockEvery.
	BlockEvery time.Duration

	once    sync.Once
	initErr error
	store   *Store
	mu      sync.Mutex // guards procs and the cache files
	procs   map[string]Process
}

func (e *Engine) init() error {
	e.once.Do(func() {
		e.procs = map[string]Process{}
		e.store, e.initErr = NewStore(e.Dir)
	})
	return e.initErr
}

// Plugin is one row of the plugin list as the API shows it.
type Plugin struct {
	ID               string  `json:"id"`
	Version          string  `json:"version"`
	Installed        bool    `json:"installed"`
	InstalledVersion string  `json:"installed_version,omitempty"`
	Running          bool    `json:"running"`
	Enabled          bool    `json:"enabled"`
	DisabledReason   string  `json:"disabled_reason,omitempty"`
	Access           *Access `json:"access"`
}

// Overview is the answer of the plugin list route.
type Overview struct {
	KeySet  bool         `json:"key_set"`
	CanRun  bool         `json:"can_run"`
	Limits  ActiveLimits `json:"limits"`
	Plugins []Plugin     `json:"plugins"`
}

// List shows the plugins of the cached signed list (fetched once when none
// is cached) and every installed one.
func (e *Engine) List(ctx context.Context) (*Overview, error) {
	if err := e.init(); err != nil {
		return nil, err
	}
	st, err := e.store.Load()
	if err != nil {
		return nil, err
	}
	ov := &Overview{KeySet: len(e.Keys) > 0, CanRun: e.Runner != nil, Limits: e.Limits, Plugins: []Plugin{}}
	rows := map[string]*Plugin{}
	var order []string
	if raw := e.cachedList(ctx); raw != nil {
		if l, _, err := e.verify(raw, KindPlugins, 0); err == nil {
			for _, en := range l.Entries {
				if p, ok := rows[en.ID]; ok {
					if newerVersion(en.Version, p.Version) {
						p.Version = en.Version
					}
					continue
				}
				rows[en.ID] = &Plugin{ID: en.ID, Version: en.Version}
				order = append(order, en.ID)
			}
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for id, in := range st.Plugins {
		p, ok := rows[id]
		if !ok {
			p = &Plugin{ID: id, Version: in.Version}
			rows[id] = p
			order = append(order, id)
		}
		p.Installed, p.InstalledVersion, p.Enabled, p.DisabledReason = true, in.Version, in.Enabled, in.DisabledReason
		_, p.Running = e.procs[id]
		if m := e.readManifest(id); m != nil && m.Version == in.Version {
			p.Access = &m.Access
		}
	}
	for _, id := range order {
		ov.Plugins = append(ov.Plugins, *rows[id])
	}
	return ov, nil
}

// cachedList is the cached plugin list, fetched once when none is cached.
func (e *Engine) cachedList(ctx context.Context) []byte {
	if raw, err := os.ReadFile(filepath.Join(e.Dir, cacheList)); err == nil {
		return raw
	}
	if len(e.Keys) == 0 {
		return nil
	}
	raw, err := e.fetch(ctx, "plugins.json")
	if err != nil {
		return nil
	}
	if _, _, err := e.verify(raw, KindPlugins, 0); err != nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	_ = writeFileSynced(filepath.Join(e.Dir, cacheList), raw)
	return raw
}

// Install fetches the lists, the manifest and the file of the newest listed
// version of id, checks them, installs and starts the plugin.
func (e *Engine) Install(ctx context.Context, id string) (*Installed, error) {
	if err := e.init(); err != nil {
		return nil, err
	}
	if len(e.Keys) == 0 {
		return nil, ErrNoKey
	}
	if e.Runner == nil {
		return nil, ErrNotLinux
	}
	if !idRe.MatchString(id) {
		return nil, fmt.Errorf("%w: %s", ErrNotListed, id)
	}
	st, err := e.store.Load()
	if err != nil {
		return nil, err
	}
	listRaw, err := e.fetch(ctx, "plugins.json")
	if err != nil {
		return nil, err
	}
	l, key, err := e.verify(listRaw, KindPlugins, st.ListCounter)
	if err != nil {
		return nil, fmt.Errorf("plugin list: %w", err)
	}
	var entry *Entry
	for i, en := range l.Entries {
		if en.ID == id && (entry == nil || newerVersion(en.Version, entry.Version)) {
			entry = &l.Entries[i]
		}
	}
	if entry == nil {
		return nil, fmt.Errorf("%w: %s", ErrNotListed, id)
	}
	blockRaw, err := e.fetch(ctx, "blocklist.json")
	if err != nil {
		return nil, err
	}
	base := "plugins/" + url.PathEscape(id) + "/" + url.PathEscape(entry.Version) + "/"
	manRaw, err := e.fetch(ctx, base+"manifest.json")
	if err != nil {
		return nil, err
	}
	file, err := e.fetchBody(ctx, base+"plugin", maxFileSize)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	acc, err := CheckInstall(InstallInput{PublicKey: key, List: listRaw, LastListCounter: st.ListCounter,
		Blocklist: blockRaw, LastBlockCounter: st.BlockCounter, Manifest: manRaw, File: file})
	if err != nil {
		return nil, err
	}
	if acc.Manifest.ID != id {
		return nil, fmt.Errorf("%w: manifest names %s", ErrNotListed, acc.Manifest.ID)
	}
	e.mu.Lock()
	if p := e.procs[id]; p != nil {
		_ = p.Stop()
		delete(e.procs, id)
	}
	_ = writeFileSynced(filepath.Join(e.Dir, cacheList), listRaw)
	_ = writeFileSynced(filepath.Join(e.Dir, cacheBlocklist), blockRaw)
	e.mu.Unlock()
	if err := e.store.Install(acc); err != nil {
		return nil, err
	}
	e.saveManifest(id, manRaw)
	if err := e.Start(ctx, id); err != nil {
		return nil, err
	}
	return e.installed(id)
}

// Start enables id and starts it after the start checks.
func (e *Engine) Start(ctx context.Context, id string) error {
	if err := e.init(); err != nil {
		return err
	}
	if e.Runner == nil {
		return ErrNotLinux
	}
	if err := e.store.SetEnabled(id, true, ""); err != nil {
		return err
	}
	return e.start(id)
}

// start runs id with the cached lists. The process lives with the app, not
// with the request that started it.
func (e *Engine) start(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.procs[id]; ok {
		return nil
	}
	in, err := e.startInput()
	if err != nil {
		return err
	}
	p, err := e.store.Start(context.Background(), e.Runner, id, in, nil, nil)
	if err != nil {
		return err
	}
	e.procs[id] = p
	go func() {
		_ = p.Wait()
		e.mu.Lock()
		if e.procs[id] == p {
			delete(e.procs, id)
		}
		e.mu.Unlock()
	}()
	return nil
}

// startInput reads the cached lists and picks the key they verify with.
// Called with e.mu held.
func (e *Engine) startInput() (StartInput, error) {
	list, err1 := os.ReadFile(filepath.Join(e.Dir, cacheList))
	block, err2 := os.ReadFile(filepath.Join(e.Dir, cacheBlocklist))
	if err := errors.Join(err1, err2); err != nil {
		return StartInput{}, fmt.Errorf("cached plugin lists: %w", err)
	}
	_, key, err := e.verify(list, KindPlugins, 0)
	if err != nil {
		key = nil
		if len(e.Keys) > 0 {
			key = e.Keys[0] // CheckStart refuses and disables with the reason
		}
	}
	if key == nil {
		return StartInput{}, ErrNoKey
	}
	return StartInput{PublicKey: key, List: list, Blocklist: block}, nil
}

// Stop ends id and keeps it disabled until started again.
func (e *Engine) Stop(id string) error {
	if err := e.init(); err != nil {
		return err
	}
	if err := e.stopProc(id); err != nil {
		return err
	}
	return e.store.SetEnabled(id, false, "stopped by the owner")
}

func (e *Engine) stopProc(id string) error {
	e.mu.Lock()
	p := e.procs[id]
	delete(e.procs, id)
	e.mu.Unlock()
	if p == nil {
		return nil
	}
	return p.Stop()
}

// Remove stops id and deletes it, or with keep disables it and keeps its
// program and data.
func (e *Engine) Remove(id string, keep bool) error {
	if err := e.init(); err != nil {
		return err
	}
	mode := RemoveDelete
	if keep {
		mode = RemoveKeep
	}
	if _, err := e.installed(id); err != nil {
		return err
	}
	// Stop before Remove takes the store lock: start holds e.mu then the
	// store lock, so the other order could deadlock.
	if err := e.stopProc(id); err != nil {
		return fmt.Errorf("stopping plugin: %w", err)
	}
	if err := e.store.Remove(id, mode); err != nil {
		return err
	}
	if !keep {
		os.Remove(filepath.Join(e.Dir, manifestDir, id+".json"))
	}
	return nil
}

// StartEnabled starts every enabled plugin at app start. A plugin that
// fails its start checks is disabled by them and logged.
func (e *Engine) StartEnabled() {
	if _, err := os.Stat(e.Dir); errors.Is(err, os.ErrNotExist) {
		return // nothing installed yet
	}
	if err := e.init(); err != nil {
		log.Printf("plugins: %v", err)
		return
	}
	st, err := e.store.Load()
	if err != nil {
		log.Printf("plugins: %v", err)
		return
	}
	for id, p := range st.Plugins {
		if !p.Enabled {
			continue
		}
		if e.Runner == nil {
			log.Printf("plugins: %s not started: %v", id, ErrNotLinux)
			continue
		}
		if err := e.start(id); err != nil {
			log.Printf("plugins: %s not started, disabled: %v", id, err)
		}
	}
}

// StopAll ends every running plugin (app shutdown).
func (e *Engine) StopAll() {
	if e.init() != nil {
		return
	}
	e.mu.Lock()
	ids := make([]string, 0, len(e.procs))
	for id := range e.procs {
		ids = append(ids, id)
	}
	e.mu.Unlock()
	for _, id := range ids {
		_ = e.stopProc(id)
	}
}

// Run checks the blocklist every BlockEvery until ctx ends.
func (e *Engine) Run(ctx context.Context) {
	every := e.BlockEvery
	if every <= 0 {
		every = DefaultBlockEvery
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := e.CheckBlocklist(ctx); err != nil {
				log.Printf("plugins: blocklist check: %v", err)
			}
		}
	}
}

// CheckBlocklist fetches both lists and stops and disables every enabled
// plugin they no longer allow. A fetch or signature failure changes nothing.
func (e *Engine) CheckBlocklist(ctx context.Context) error {
	if err := e.init(); err != nil {
		return err
	}
	st, err := e.store.Load()
	if err != nil {
		return err
	}
	if len(st.Plugins) == 0 || len(e.Keys) == 0 {
		return nil
	}
	listRaw, err := e.fetch(ctx, "plugins.json")
	if err != nil {
		return err
	}
	_, key, err := e.verify(listRaw, KindPlugins, st.ListCounter)
	if err != nil {
		return fmt.Errorf("plugin list: %w", err)
	}
	blockRaw, err := e.fetch(ctx, "blocklist.json")
	if err != nil {
		return err
	}
	if _, _, err := e.verify(blockRaw, KindBlocklist, st.BlockCounter); err != nil {
		return fmt.Errorf("blocklist: %w", err)
	}
	e.mu.Lock()
	_ = writeFileSynced(filepath.Join(e.Dir, cacheList), listRaw)
	_ = writeFileSynced(filepath.Join(e.Dir, cacheBlocklist), blockRaw)
	e.mu.Unlock()
	for id, p := range st.Plugins {
		if !p.Enabled {
			continue
		}
		if _, err := e.store.CheckStart(id, StartInput{PublicKey: key, List: listRaw, Blocklist: blockRaw}); err != nil {
			log.Printf("plugins: %s stopped and disabled: %v", id, err)
			if serr := e.stopProc(id); serr != nil {
				log.Printf("plugins: stopping %s: %v", id, serr)
			}
		}
	}
	return nil
}

// verify checks raw with each pinned key and returns the key that signed it.
func (e *Engine) verify(raw []byte, kind string, last uint64) (*List, ed25519.PublicKey, error) {
	if len(e.Keys) == 0 {
		return nil, nil, ErrNoKey
	}
	var first error
	for _, k := range e.Keys {
		l, err := VerifyList(k, raw, kind, last)
		if err == nil {
			return l, k, nil
		}
		if first == nil || !errors.Is(err, ErrListSignature) {
			first = err
		}
	}
	return nil, nil, first
}

func (e *Engine) installed(id string) (*Installed, error) {
	st, err := e.store.Load()
	if err != nil {
		return nil, err
	}
	p, ok := st.Plugins[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotInstalled, id)
	}
	return p, nil
}

func (e *Engine) saveManifest(id string, raw []byte) {
	dir := filepath.Join(e.Dir, manifestDir)
	if err := os.MkdirAll(dir, 0o700); err == nil {
		_ = writeFileSynced(filepath.Join(dir, id+".json"), raw)
	}
}

func (e *Engine) readManifest(id string) *Manifest {
	raw, err := os.ReadFile(filepath.Join(e.Dir, manifestDir, id+".json"))
	if err != nil {
		return nil
	}
	m, err := ParseManifest(raw)
	if err != nil {
		return nil
	}
	return m
}

func (e *Engine) fetch(ctx context.Context, rel string) ([]byte, error) {
	body, err := e.fetchBody(ctx, rel, maxListSize)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	data, err := io.ReadAll(io.LimitReader(body, maxListSize+1))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFetch, err)
	}
	if len(data) > maxListSize {
		return nil, fmt.Errorf("%w: %s is too large", ErrFetch, rel)
	}
	return data, nil
}

// fetchBody GETs rel below ListURL over https; certificates are always
// verified. The caller closes the body.
func (e *Engine) fetchBody(ctx context.Context, rel string, max int64) (io.ReadCloser, error) {
	base, err := url.Parse(e.ListURL)
	if err != nil || base.Scheme != "https" || base.Host == "" {
		return nil, fmt.Errorf("%w: list address %q is not https", ErrFetch, e.ListURL)
	}
	if !strings.HasSuffix(base.Path, "/") {
		base.Path += "/"
	}
	u, err := base.Parse(rel)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFetch, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFetch, err)
	}
	c := e.HTTP
	if c == nil {
		c = &http.Client{Timeout: 5 * time.Minute}
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFetch, err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("%w: %s answered %d", ErrFetch, rel, resp.StatusCode)
	}
	if resp.ContentLength > max {
		resp.Body.Close()
		return nil, fmt.Errorf("%w: %s is too large", ErrFetch, rel)
	}
	return resp.Body, nil
}

// newerVersion compares plain semver cores; a pre-release sorts below its
// release.
func newerVersion(a, b string) bool {
	ca, pa, _ := strings.Cut(a, "-")
	cb, pb, _ := strings.Cut(b, "-")
	na, nb := strings.Split(ca, "."), strings.Split(cb, ".")
	for i := 0; i < 3 && i < len(na) && i < len(nb); i++ {
		if len(na[i]) != len(nb[i]) {
			return len(na[i]) > len(nb[i])
		}
		if na[i] != nb[i] {
			return na[i] > nb[i]
		}
	}
	if (pa == "") != (pb == "") {
		return pa == ""
	}
	return pa > pb
}
