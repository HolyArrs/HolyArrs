package plugins

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
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
	// and .../plugin below it; other platforms use their own list and folder
	// (see ListName and FileBase).
	ListURL string
	// Platform picks the build to install, such as "linux-arm64"; "" is the
	// running machine's (HostPlatform).
	Platform string
	// Keys are the pinned public keys; empty refuses every install.
	Keys []ed25519.PublicKey
	// Runner starts plugins; nil means plugins cannot run here (not Linux).
	Runner Runner
	// NoFileLock is true when this system cannot give plugins the Landlock
	// file lock. A plugin then starts only when the owner allowed it.
	NoFileLock bool
	// FileLockReason says why: FileLockOldKernel, FileLockBlocked or
	// FileLockTurnedOff.
	FileLockReason string
	// NoNetLock and NetLockReason are the same for the network lock (netlock.go);
	// Proxy is the plugins' way out to their hosts, nil for a default one.
	NoNetLock     bool
	NetLockReason string
	Proxy         *Proxy
	// HTTP fetches lists and files; nil uses a client with a timeout.
	HTTP *http.Client
	// BlockEvery is the blocklist check interval; 0 means DefaultBlockEvery.
	BlockEvery time.Duration
	// Replay lists the history events of the given plugin event names with
	// an id above after, oldest first, at most limit (catch-up after a
	// stop); LastEventID is the newest history id. Either may be nil.
	Replay      func(ctx context.Context, events []string, after int64, limit int) []Replayed
	LastEventID func(ctx context.Context) (int64, error)
	// Env gives the HOLYARRS_PLUGIN_* settings for a plugin's start; may be nil.
	Env func(id string) []string
	// Forwards says whether id hands visits in from outside; may be nil.
	Forwards func(id string) bool
	// Revoke ends what a start was handed (its API token); may be nil.
	Revoke func(id string)
	// Token hands a start its API token and the app address, for its scopes.
	Token func(id string, scopes []string) []string
	// AppVersion is this app's version; a plugin whose min_app_version is
	// newer is listed with needs_app and refused at install. "", "dev" or an
	// unstamped 0.0.0-* build skips the check.
	AppVersion string
	// SettingsKey seals the secret fields of plugin settings; nil refuses
	// saving a secret. Derived from the app's master key.
	SettingsKey []byte

	once      sync.Once
	proxyOnce sync.Once
	initErr   error
	store     *Store
	mu        sync.Mutex // guards procs and the cache files
	procs     map[string]Process
	// feeds are the running plugins' event streams (events.go).
	feedMu sync.Mutex
	feeds  map[string]*feed
	// steps is each running install's current step (downloading, checking,
	// starting); fails is the short reason of each plugin's last failed
	// install, until the next try. Both guarded by mu.
	steps map[string]string
	fails map[string]string
	// opMu covers a whole install, start, stop or remove of a plugin, so a
	// start never slips in between an install's stop and its start.
	opMu sync.Mutex
	// installing lets one install download and check at a time.
	installing chan struct{}
	// statData is the last stats.json, fetched at most hourly (statAt).
	statMu   sync.Mutex
	statData map[string]Stat
	statAt   time.Time
	statBusy bool
	// statFirst closes when the running stats fetch ends.
	statFirst chan struct{}
}

func (e *Engine) init() error {
	e.once.Do(func() {
		e.procs = map[string]Process{}
		e.steps, e.fails = map[string]string{}, map[string]string{}
		e.installing = make(chan struct{}, 1)
		e.store, e.initErr = NewStore(e.Dir)
		if e.initErr == nil {
			e.dropKept()
			e.keepForwarding()
		}
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
	Name        string `json:"name,omitempty"`
	Author      string `json:"author,omitempty"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
	IconURL     string `json:"icon_url,omitempty"`
	Details
	Access *Access `json:"access"`
	// DownloadTypes are the download types the plugin brings itself.
	DownloadTypes []string `json:"download_types,omitempty"`
	// ReachesApp is true when the plugin is handed this server's own address.
	ReachesApp bool `json:"reaches_app"`
	// InstallStep is the step of an install under way: downloading,
	// checking or starting.
	InstallStep string `json:"install_step,omitempty"`
	// InstallError is why the last install failed, until the next try.
	InstallError string `json:"install_error,omitempty"`
	// InstalledAccess is what the installed version may reach, when a newer
	// version is listed, so the page can show what an update adds.
	InstalledAccess *Access `json:"installed_access,omitempty"`
	// NeedsApp is the app version the listed version needs, when newer
	// than this app.
	NeedsApp string `json:"needs_app,omitempty"`
	// Stats are the public download and star numbers, when known.
	Stats *Stat `json:"stats,omitempty"`
	// NeedsApproval, Grants, OwnerHosts and the blocked counts: see showAccess.
	NeedsApproval []string               `json:"needs_approval,omitempty"`
	Grants        map[string]FolderGrant `json:"grants,omitempty"`
	OwnerHosts    []string               `json:"owner_hosts,omitempty"`
	BlockedCount  int                    `json:"blocked_count,omitempty"`
	BlockedRecent []BlockedHit           `json:"blocked_recent,omitempty"`
	// ScopeGroups are the listed version's API scopes as install lines.
	ScopeGroups []ScopeGroup `json:"scope_groups,omitempty"`
	// Resources are the installed version's asked caps; OwnerLimits the
	// owner's own.
	Resources   *Resources `json:"resources,omitempty"`
	OwnerLimits *Resources `json:"owner_limits,omitempty"`
	// Dev is a plugin in the developer folder (installed from it, or
	// ready to be); DevError says why its files can't be read.
	Dev      bool   `json:"dev,omitempty"`
	DevError string `json:"dev_error,omitempty"`
	// Test is an installed test version. TestVersion is the newest test
	// version, when newer than the newest normal one; TestOn is the
	// owner's choice to get test versions of this plugin.
	Test        bool   `json:"test,omitempty"`
	TestVersion string `json:"test_version,omitempty"`
	TestOn      bool   `json:"test_on,omitempty"`
}

// show copies a manifest's display fields and access onto the row.
func (p *Plugin) show(m *Manifest) {
	p.Name, p.Author, p.Description, p.Icon, p.IconURL, p.Details = m.Name, m.Author, m.Description, m.Icon, m.IconURL, m.Details
	a := m.Access
	p.Access, p.DownloadTypes = &a, m.DownloadTypes
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
	// NoFileLock is true when this system cannot give plugins the file lock.
	NoFileLock bool `json:"no_file_lock"`
	// AllowNoFileLock is the owner's install-wide choice to let signed
	// plugins run without the file lock; NoFileLockAsked is true once answered.
	AllowNoFileLock bool `json:"allow_no_file_lock"`
	NoFileLockAsked bool `json:"no_file_lock_asked"`
	// FileLockReason says why (old_kernel, blocked, turned_off); empty with the lock.
	FileLockReason string `json:"file_lock_reason,omitempty"`
	NoNetLock      bool   `json:"no_net_lock"` // the same four for the network lock
	AllowNoNetLock bool   `json:"allow_no_net_lock"`
	NoNetLockAsked bool   `json:"no_net_lock_asked"`
	NetLockReason  string `json:"net_lock_reason,omitempty"`
	// ListBad is true when the plugin list arrived but failed its signature
	// check. A list that can't be reached (no address yet, 404) is not bad:
	// the page just shows no plugins.
	ListBad bool `json:"list_bad"`
	// DevMode is on when the owner runs unsigned plugins from DevDir.
	DevMode bool   `json:"dev_mode"`
	DevDir  string `json:"dev_dir,omitempty"`
	// LanguagePacks are the language packs' download numbers, when known;
	// that card is not a listed plugin.
	LanguagePacks *Stat    `json:"language_packs,omitempty"`
	Plugins       []Plugin `json:"plugins"`
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
	ov := &Overview{KeySet: len(e.Keys) > 0, CanRun: e.Runner != nil, NoFileLock: e.Runner != nil && e.NoFileLock, Plugins: []Plugin{}}
	if ov.NoFileLock {
		ov.FileLockReason = e.FileLockReason
		if ov.FileLockReason == "" {
			ov.FileLockReason = FileLockOldKernel
		}
	}
	ov.AllowNoFileLock, ov.NoFileLockAsked = st.AllowNoFileLock, st.NoFileLockAsked
	if ov.DevMode = st.DevMode; ov.DevMode {
		ov.DevDir = e.DevDir()
	}
	e.showNetLock(ov, st)
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
	e.showTestVersions(ctx, st, rows, newest)
	for id, en := range newest {
		rows[id].Version = en.Version
		rows[id].TestOn = st.TestVersions[id]
		if m := e.listedManifest(ctx, en); m != nil {
			rows[id].show(m)
			if e.tooOld(m.MinAppVersion) {
				rows[id].NeedsApp = m.MinAppVersion
			}
		}
	}
	stats := e.stats()
	e.mu.Lock()
	defer e.mu.Unlock()
	// Scopes an installed plugin was granted outside its manifest (the 1.3.0 upgrade's network.forward for an old
	// Tailscale): they count for the install groups and their red warnings too.
	kept := map[string][]string{}
	for id, in := range st.Plugins {
		if in.Approval != nil {
			kept[id] = in.Approval.Kept
		}
		p, ok := rows[id]
		if !ok {
			p = &Plugin{ID: id, Version: in.Version}
			rows[id] = p
			order = append(order, id)
		}
		p.Installed, p.InstalledVersion, p.Enabled, p.DisabledReason = true, in.Version, in.Enabled, in.DisabledReason
		p.Dev, p.Test = in.Dev, in.Test
		if in.Dev {
			p.Version = in.Version
		}
		_, p.Running = e.procs[id]
		im := e.readManifest(id)
		e.showAccess(p, in, im)
		if m := im; m != nil && m.Version == in.Version {
			if p.Version == in.Version {
				p.show(m)
			} else {
				a := m.Access
				p.InstalledAccess = &a
			}
		}
	}
	order = e.showDev(st, rows, order)
	if s, ok := stats["language-packs"]; ok {
		ov.LanguagePacks = &s
	}
	for _, id := range order {
		rows[id].InstallStep, rows[id].InstallError = e.steps[id], e.fails[id]
		rows[id].ReachesApp = e.Forwards != nil && e.Forwards(id)
		rows[id].ScopeGroups = scopeGroups(withKept(rows[id].Access, kept[id]))
		if s, ok := stats[id]; ok {
			s := s
			rows[id].Stats = &s
		}
		ov.Plugins = append(ov.Plugins, *rows[id])
	}
	return ov, nil
}

// cachedList is the cached plugin list, fetched once when none is cached.
// bad is true when a fetched list fails its signature check; a list that
// can't be fetched is just absent.
func (e *Engine) cachedList(ctx context.Context) (raw []byte, bad bool) {
	if raw, err := os.ReadFile(filepath.Join(e.Dir, e.cacheListName())); err == nil {
		return raw, false
	}
	if len(e.Keys) == 0 {
		return nil, false
	}
	raw, err := e.fetch(ctx, ListName(e.platform()))
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
	_ = writeFileSynced(filepath.Join(e.Dir, e.cacheListName()), raw)
	return raw, false
}

// Install fetches the lists, the manifest and the file of the newest listed
// version of id, checks them, installs and starts the plugin. A first
// install approves the hosts the owner saw and the folders in approve (nil:
// none); an update keeps what was approved and is held at start when it
// asks for more.
func (e *Engine) Install(ctx context.Context, id string, approve ...ApproveInput) (*Installed, error) {
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
	e.setStep(id, "downloading")
	var ap *ApproveInput
	if len(approve) > 0 {
		ap = &approve[0]
	}
	inst, err := e.install(ctx, id, ap)
	e.mu.Lock()
	delete(e.steps, id)
	if err != nil {
		e.fails[id] = shortReason(err)
	}
	e.mu.Unlock()
	return inst, err
}

// setStep records id's install step and clears its last failure.
func (e *Engine) setStep(id, step string) {
	e.mu.Lock()
	e.steps[id] = step
	delete(e.fails, id)
	e.mu.Unlock()
}

// tooOld is true when this app is older than min. A dev build or an
// unknown version skips the check.
func (e *Engine) tooOld(min string) bool {
	app := strings.TrimPrefix(e.AppVersion, "v")
	// An unstamped build (dev server, lab image built without -ldflags) reports 0.0.0-something; skip the check there.
	if !semverRe.MatchString(app) || !semverRe.MatchString(min) || strings.HasPrefix(app, "0.0.0") {
		return false
	}
	return newerVersion(min, app)
}

// shortReason is a failed install's error in one short line.
func shortReason(err error) string {
	s := err.Error()
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 160 {
		s = s[:157] + "..."
	}
	return s
}

// stepReader marks the install as checking once the file is fully read.
type stepReader struct {
	r      io.Reader
	onDone func()
	done   bool
}

func (s *stepReader) Read(b []byte) (int, error) {
	n, err := s.r.Read(b)
	if err == io.EOF && !s.done {
		s.done = true
		s.onDone()
	}
	return n, err
}

func (e *Engine) install(ctx context.Context, id string, ap *ApproveInput) (*Installed, error) {
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
	listRaw, err := e.fetch(ctx, ListName(e.platform()))
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
	kind, checkRaw, lastCounter := KindPlugins, listRaw, st.ListCounter
	if st.TestVersions[id] {
		if testRaw := e.testList(ctx, st, true); testRaw != nil {
			if tl, _, err := e.verify(testRaw, KindTest, st.TestCounter); err == nil {
				for i, en := range tl.Entries {
					if en.ID == id && (entry == nil || newerVersion(en.Version, entry.Version)) {
						entry, kind, checkRaw, lastCounter = &tl.Entries[i], KindTest, testRaw, st.TestCounter
					}
				}
			}
		}
	}
	if entry == nil {
		return nil, fmt.Errorf("%w: %s", ErrNotListed, id)
	}
	blockRaw, err := e.fetch(ctx, "blocklist.json")
	if err != nil {
		return nil, err
	}
	base := FileBase(id, entry.Version, e.platform())
	manRaw, err := e.fetch(ctx, base+"manifest.json")
	if err != nil {
		return nil, err
	}
	fileRel := base + "plugin"
	if entry.FileURL != "" {
		fileRel = entry.FileURL
	}
	file, err := e.fetchBody(ctx, fileRel, maxFileSize)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	acc, err := CheckInstall(InstallInput{PublicKey: key, List: checkRaw, LastListCounter: lastCounter, Kind: kind,
		Blocklist: blockRaw, LastBlockCounter: st.BlockCounter, Manifest: manRaw, TempDir: e.Dir,
		File: &stepReader{r: file, onDone: func() { e.setStep(id, "checking") }}})
	if err != nil {
		return nil, err
	}
	defer acc.Close()
	if acc.Manifest.ID != id {
		return nil, fmt.Errorf("%w: manifest names %s", ErrNotListed, acc.Manifest.ID)
	}
	if e.tooOld(acc.Manifest.MinAppVersion) {
		return nil, fmt.Errorf("%w: needs HolyArrs %s or newer", ErrNeedsNewerApp, acc.Manifest.MinAppVersion)
	}
	if err := e.checkPublisher(id, key); err != nil {
		return nil, err
	}
	e.opMu.Lock()
	defer e.opMu.Unlock()
	if err := e.writeCache(listRaw, blockRaw, l.Counter, acc.BlockCounter); err != nil {
		return nil, err
	}
	if held, err := e.holdUpdate(id, acc, manRaw); held {
		return nil, err // the running version keeps running until the owner approves
	}
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
	_ = e.store.SetPending(id, nil)
	_ = os.RemoveAll(e.store.pendingDir(id))
	if err := e.store.SetKey(id, keyHex(key)); err != nil {
		return nil, err
	}
	e.saveManifest(id, manRaw)
	if fresh {
		if err := e.firstApproval(id, ap); err != nil {
			return nil, err
		}
	}
	in, err := e.installed(id)
	if err != nil || !in.Enabled {
		return in, err // an update of a plugin the owner stopped stays stopped
	}
	e.setStep(id, "starting")
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
	var in StartInput
	if st, _ := e.store.Load(); st == nil || st.Plugins[id] == nil || !st.Plugins[id].Dev {
		var err error
		if in, err = e.startInput(); err != nil {
			_ = e.store.SetEnabled(id, false, err.Error())
			return err
		}
	}
	e.backfillKey(id, in.PublicKey)
	var env []string
	if e.Env != nil {
		env = e.Env(id)
	}
	env = append(env, e.tokenEnv(id, env)...)
	env = append(env, e.settingsEnv(id)...)
	acc, err := e.startAccess(id, env)
	if err != nil {
		e.revoke(id)
		_ = e.store.SetEnabled(id, false, err.Error())
		return err
	}
	fd, after, err := e.prepareFeed(id, &acc)
	if err != nil {
		for _, f := range acc.Folders {
			f.File.Close()
		}
		if acc.Proxy != nil {
			acc.Proxy.Close()
		}
		return err
	}
	p, err := e.store.Start(context.Background(), e.Runner, id, in, nil, env, acc)
	for _, f := range acc.Folders {
		f.File.Close() // the plugin has its own copy
	}
	if err != nil {
		e.abandonFeed(fd, acc)
		if acc.Proxy != nil {
			acc.Proxy.Close()
		}
		e.revoke(id)
		_ = e.store.SetEnabled(id, false, err.Error())
		return err
	}
	if acc.Proxy != nil && e.NoNetLock {
		log.Printf("plugins: %s started WITHOUT the network lock (%s): the owner allowed it on the Plugins page; UDP and raw sockets stay blocked", id, e.NetLockReason)
	}
	if e.NoFileLock {
		log.Printf("plugins: %s started WITHOUT the file lock (%s): the owner allowed it on the Plugins page; memory, process and CPU limits stay on", id, e.FileLockReason)
	}
	e.procs[id] = p
	e.attachFeed(id, fd, after)
	go func() {
		werr := p.Wait()
		e.dropFeed(id, fd)
		if acc.Proxy != nil {
			acc.Proxy.Close()
		}
		e.mu.Lock()
		own := e.procs[id] == p // still listed: it ended on its own, not by Stop
		if own {
			delete(e.procs, id)
			e.revoke(id)
		}
		e.mu.Unlock()
		if own {
			e.exited(id, werr)
		}
	}()
	return nil
}

// exited records a plugin that ended on its own: it is logged and turned
// off with the reason, so the page shows why and offers Start. It is not
// restarted, so a crashing plugin never loops.
func (e *Engine) exited(id string, err error) {
	why := "stopped on its own"
	if err != nil {
		why += " (" + err.Error() + ")"
	}
	log.Printf("plugins: %s %s", id, why)
	e.opMu.Lock()
	defer e.opMu.Unlock()
	e.mu.Lock()
	_, again := e.procs[id] // started again meanwhile
	e.mu.Unlock()
	if !again {
		_ = e.store.SetEnabled(id, false, why)
	}
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
	return e.store.SetEnabled(id, false, stoppedByOwner)
}

// Remove stops id and deletes it with its settings and data.
func (e *Engine) Remove(id string) error {
	if err := e.init(); err != nil {
		return err
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
	if err := e.store.Remove(id, RemoveDelete); err != nil {
		return err
	}
	os.Remove(filepath.Join(e.Dir, manifestDir, id+".json"))
	return nil
}

// dropKept wipes plugins an earlier build removed but kept the data of, so
// they show as not installed and can be installed again.
func (e *Engine) dropKept() {
	st, err := e.store.Load()
	if err != nil {
		return
	}
	for id, p := range st.Plugins {
		if p.Enabled || p.DisabledReason != keptReason {
			continue
		}
		if err := e.store.Remove(id, RemoveDelete); err != nil {
			log.Printf("plugins: clearing removed %s: %v", id, err)
			continue
		}
		os.Remove(filepath.Join(e.Dir, manifestDir, id+".json"))
	}
}

// StartEnabled starts every enabled plugin at app start. A plugin that
// fails its start checks is disabled by them and logged.
func (e *Engine) StartEnabled() {
	e.prefetchStats() // the Plugins page shows the numbers on its first view
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
