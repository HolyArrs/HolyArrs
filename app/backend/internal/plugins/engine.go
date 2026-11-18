package plugins

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
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
	// listManifestDir caches the manifests of listed (not yet installed)
	// versions, each checked against the signed list's manifest hash.
	listManifestDir = ".list-manifests"
)

// maxListSize caps a fetched list or manifest.
const maxListSize = 4 << 20

// DefaultBlockEvery is how often the blocklist is checked.
const DefaultBlockEvery = 6 * time.Hour

// blockStaleAfter is when the list shows the blocklist check as stale. It
// only warns: signatures never expire and nothing is stopped for it.
const blockStaleAfter = 7 * 24 * time.Hour

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
	// HTTP fetches lists and files; nil uses a client with a timeout.
	HTTP *http.Client
	// BlockEvery is the blocklist check interval; 0 means DefaultBlockEvery.
	BlockEvery time.Duration
	// Env gives the HOLYARRS_PLUGIN_* settings for a plugin's start; may be nil.
	Env func(id string) []string

	once    sync.Once
	initErr error
	store   *Store
	mu      sync.Mutex // guards procs and the cache files
	procs   map[string]Process
	// opMu covers a whole install, start, stop or remove of a plugin, so a
	// start never slips in between an install's stop and its start.
	opMu sync.Mutex
	// installing lets one install download and check at a time.
	installing chan struct{}
}

func (e *Engine) init() error {
	e.once.Do(func() {
		e.procs = map[string]Process{}
		e.installing = make(chan struct{}, 1)
		e.store, e.initErr = NewStore(e.Dir)
	})
	return e.initErr
}

// Plugin is one row of the plugin list as the API shows it.
type Plugin struct {
	ID               string `json:"id"`
	Version          string `json:"version"`
	Installed        bool   `json:"installed"`
	InstalledVersion string `json:"installed_version,omitempty"`
	Running          bool   `json:"running"`
	Enabled          bool   `json:"enabled"`
	DisabledReason   string `json:"disabled_reason,omitempty"`
	// Name, Author, Description and Icon come from the signed manifest.
	Name        string  `json:"name,omitempty"`
	Author      string  `json:"author,omitempty"`
	Description string  `json:"description,omitempty"`
	Icon        string  `json:"icon,omitempty"`
	Access      *Access `json:"access"`
	// ReachesApp is true when the plugin is handed this server's own address.
	ReachesApp bool `json:"reaches_app"`
}

// show copies a manifest's display fields and access onto the row.
func (p *Plugin) show(m *Manifest) {
	p.Name, p.Author, p.Description, p.Icon = m.Name, m.Author, m.Description, m.Icon
	a := m.Access
	p.Access = &a
}

// Overview is the answer of the plugin list route.
type Overview struct {
	KeySet bool `json:"key_set"`
	// BlocklistCheckedAt is when the blocklist last verified; null before
	// the first check.
	BlocklistCheckedAt *time.Time `json:"blocklist_checked_at"`
	// BlocklistStale is true when plugins are installed and the last
	// check is older than 7 days.
	BlocklistStale bool `json:"blocklist_stale"`
	CanRun         bool `json:"can_run"`
	// ListBad is true when the plugin list arrived but failed its signature
	// check. A list that can't be reached (no address yet, 404) is not bad:
	// the page just shows no plugins.
	ListBad bool     `json:"list_bad"`
	Plugins []Plugin `json:"plugins"`
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
	ov := &Overview{KeySet: len(e.Keys) > 0, CanRun: e.Runner != nil, Plugins: []Plugin{}}
	if !st.BlockCheckedAt.IsZero() {
		at := st.BlockCheckedAt
		ov.BlocklistCheckedAt = &at
	}
	ov.BlocklistStale = len(st.Plugins) > 0 && time.Since(st.BlockCheckedAt) > blockStaleAfter
	rows := map[string]*Plugin{}
	newest := map[string]Entry{}
	var order []string
	raw, bad := e.cachedList(ctx)
	ov.ListBad = bad
	if raw != nil {
		if l, _, err := e.verify(raw, KindPlugins, 0); err != nil {
			log.Printf("plugins: cached plugin list rejected: %v", err)
			ov.ListBad = true
		} else {
			for _, en := range l.Entries {
				if p, ok := rows[en.ID]; ok {
					if newerVersion(en.Version, p.Version) {
						p.Version = en.Version
						newest[en.ID] = en
					}
					continue
				}
				rows[en.ID] = &Plugin{ID: en.ID, Version: en.Version}
				newest[en.ID] = en
				order = append(order, en.ID)
			}
		}
	}
	for id, en := range newest {
		if m := e.listedManifest(ctx, en); m != nil {
			rows[id].show(m)
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
		if m := e.readManifest(id); m != nil && m.Version == in.Version && p.Version == in.Version {
			p.show(m)
		}
	}
	for _, id := range order {
		rows[id].ReachesApp = e.Env != nil && len(e.Env(id)) > 0
		ov.Plugins = append(ov.Plugins, *rows[id])
	}
	return ov, nil
}

// cachedList is the cached plugin list, fetched once when none is cached.
// bad is true when a fetched list fails its signature check; a list that
// can't be fetched is just absent.
func (e *Engine) cachedList(ctx context.Context) (raw []byte, bad bool) {
	if raw, err := os.ReadFile(filepath.Join(e.Dir, cacheList)); err == nil {
		return raw, false
	}
	if len(e.Keys) == 0 {
		return nil, false
	}
	raw, err := e.fetch(ctx, "plugins.json")
	if err != nil {
		log.Printf("plugins: plugin list fetch failed: %v", err)
		return nil, false
	}
	if _, _, err := e.verify(raw, KindPlugins, 0); err != nil {
		log.Printf("plugins: plugin list check failed: %v", err)
		return nil, true
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	_ = writeFileSynced(filepath.Join(e.Dir, cacheList), raw)
	return raw, false
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
	select {
	case e.installing <- struct{}{}:
		defer func() { <-e.installing }()
	case <-ctx.Done():
		return nil, ctx.Err()
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
		Blocklist: blockRaw, LastBlockCounter: st.BlockCounter, Manifest: manRaw, File: file, TempDir: e.Dir})
	if err != nil {
		return nil, err
	}
	defer acc.Close()
	if acc.Manifest.ID != id {
		return nil, fmt.Errorf("%w: manifest names %s", ErrNotListed, acc.Manifest.ID)
	}
	e.opMu.Lock()
	defer e.opMu.Unlock()
	if err := e.stopProc(id); err != nil {
		return nil, fmt.Errorf("stopping plugin: %w", err)
	}
	if err := e.store.Install(acc); err != nil {
		return nil, err
	}
	e.writeCache(listRaw, blockRaw, acc.ListCounter, acc.BlockCounter)
	e.saveManifest(id, manRaw)
	if err := e.start(id); err != nil {
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
	e.opMu.Lock()
	defer e.opMu.Unlock()
	if err := e.store.SetEnabled(id, true, ""); err != nil {
		return err
	}
	return e.start(id)
}

// start runs id with the cached lists. The process lives with the app, not
// with the request that started it. A failed start disables the plugin with
// the reason.
func (e *Engine) start(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.procs[id]; ok {
		return nil
	}
	in, err := e.startInput()
	if err != nil {
		_ = e.store.SetEnabled(id, false, err.Error())
		return err
	}
	var env []string
	if e.Env != nil {
		env = e.Env(id)
	}
	p, err := e.store.Start(context.Background(), e.Runner, id, in, nil, env)
	if err != nil {
		_ = e.store.SetEnabled(id, false, err.Error())
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
	st, err := e.store.Load()
	if err != nil {
		return StartInput{}, err
	}
	if _, _, err := e.verify(block, KindBlocklist, st.BlockCounter); err != nil {
		return StartInput{}, fmt.Errorf("cached blocklist: %w", err)
	}
	_, key, err := e.verify(list, KindPlugins, st.ListCounter)
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
	e.opMu.Lock()
	defer e.opMu.Unlock()
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
	e.opMu.Lock()
	defer e.opMu.Unlock()
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
	if err := e.CheckBlocklist(ctx); err != nil {
		log.Printf("plugins: blocklist check: %v", err)
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
	l, key, err := e.verify(listRaw, KindPlugins, st.ListCounter)
	if err != nil {
		return fmt.Errorf("plugin list: %w", err)
	}
	blockRaw, err := e.fetch(ctx, "blocklist.json")
	if err != nil {
		return err
	}
	block, _, err := e.verify(blockRaw, KindBlocklist, st.BlockCounter)
	if err != nil {
		return fmt.Errorf("blocklist: %w", err)
	}
	if err := e.store.SaveCounters(l.Counter, block.Counter, time.Now()); err != nil {
		return err
	}
	e.writeCache(listRaw, blockRaw, l.Counter, block.Counter)
	// Hold opMu and reload, so an install or start during the fetch is
	// checked against the new lists too.
	e.opMu.Lock()
	defer e.opMu.Unlock()
	if st, err = e.store.Load(); err != nil {
		return err
	}
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

// writeCache writes the cached lists after their counters are stored, and
// only when they are not older than the stored ones, so a slower check
// never puts back an older list.
func (e *Engine) writeCache(listRaw, blockRaw []byte, list, block uint64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	st, err := e.store.Load()
	if err != nil || list < st.ListCounter || block < st.BlockCounter {
		return
	}
	_ = writeFileSynced(filepath.Join(e.Dir, cacheList), listRaw)
	_ = writeFileSynced(filepath.Join(e.Dir, cacheBlocklist), blockRaw)
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

// listedManifest is the manifest of a listed version, fetched once and
// cached; it counts only when its bytes match the signed list's hash and id.
func (e *Engine) listedManifest(ctx context.Context, en Entry) *Manifest {
	path := filepath.Join(e.Dir, listManifestDir, en.ID+"-"+en.Version+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		if raw, err = e.fetch(ctx, "plugins/"+en.ID+"/"+en.Version+"/manifest.json"); err != nil {
			log.Printf("plugins: manifest fetch for %s %s failed: %v", en.ID, en.Version, err)
			return nil
		}
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != en.ManifestSHA256 {
		log.Printf("plugins: manifest for %s %s does not match the list's hash", en.ID, en.Version)
		return nil
	}
	m, err := ParseManifest(raw)
	if err != nil || m.ID != en.ID || m.Version != en.Version {
		log.Printf("plugins: manifest for %s %s is not valid or names another plugin (%v)", en.ID, en.Version, err)
		return nil
	}
	if os.MkdirAll(filepath.Dir(path), 0o700) == nil {
		_ = writeFileSynced(path, raw)
	}
	return m
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
	c := http.Client{Timeout: 5 * time.Minute}
	if e.HTTP != nil {
		c = *e.HTTP
	}
	c.CheckRedirect = RedirectGuard(e.ListURL)
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

// RedirectGuard refuses any redirect that leaves https or the host of
// listURL. Every list client uses it.
func RedirectGuard(listURL string) func(*http.Request, []*http.Request) error {
	host := ""
	if u, err := url.Parse(listURL); err == nil {
		host = u.Host
	}
	return func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" || req.URL.Host != host {
			return fmt.Errorf("%w: redirect to %s refused", ErrFetch, req.URL.Redacted())
		}
		if len(via) >= 10 {
			return fmt.Errorf("%w: too many redirects", ErrFetch)
		}
		return nil
	}
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
