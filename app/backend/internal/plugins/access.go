package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
)

// Access approval refusals.
var (
	// ErrNeedsApproval means the installed version asks for access the owner
	// has not approved yet: a new host or folder, or read-write where read
	// was approved, or a required folder with none chosen.
	ErrNeedsApproval = errors.New("this plugin asks for more access: approve it on the Plugins page")
	// ErrFolderMoved means an approved folder is no longer the one the owner
	// chose (it was removed, replaced or turned into a link).
	ErrFolderMoved = errors.New("a folder this plugin uses moved or changed: choose it again")
	// ErrFolderRefused marks a folder the owner may not hand to a plugin.
	ErrFolderRefused = errors.New("this folder can't be given to a plugin")
)

// Approval is what the owner approved for one installed plugin.
type Approval struct {
	// Hosts are the listed hosts the owner saw and approved.
	Hosts []string `json:"hosts"`
	// Folders are the folder requests the owner saw, with the mode seen.
	Folders []FolderReq `json:"folders"`
	// Grants maps a folder name to the real folder the owner chose.
	Grants map[string]FolderGrant `json:"grants,omitempty"`
	// OwnerHosts are hosts the owner added ("host" or "host:port"), up to 100;
	// they may reach LAN addresses (a self-hosted Headscale).
	OwnerHosts []string `json:"owner_hosts,omitempty"`
	// Scopes are the API scopes (access.actions) the owner approved.
	Scopes []string `json:"scopes,omitempty"`
	// Kept are scopes the owner never saw a manifest ask for, kept from
	// before 1.3.0 so an upgrade doesn't break what already worked (the
	// Tailscale plugin forwarding). A newer version that asks for them
	// itself replaces them.
	Kept []string `json:"kept,omitempty"`
	// Resources are the caps above the defaults the owner approved, and
	// OwnerLimits the caps the owner set, which always win.
	Resources   *Resources `json:"resources,omitempty"`
	OwnerLimits *Resources `json:"owner_limits,omitempty"`
	// Events are the events the owner approved (see EventNames).
	Events []string `json:"events,omitempty"`
}

// FolderGrant is one approved folder: its real path and the device and
// inode it had, checked again at every start.
type FolderGrant struct {
	Path       string    `json:"path"`
	Mode       string    `json:"mode"`
	Dev        uint64    `json:"dev"`
	Ino        uint64    `json:"ino"`
	ApprovedAt time.Time `json:"approved_at"`
}

// approvalFor is a's approval, or for installs from before 1.2.1 (none
// stored) the hosts the owner saw at install and no folders.
func approvalFor(in *Installed, m *Manifest) Approval {
	if in != nil && in.Approval != nil {
		return *in.Approval
	}
	a := Approval{}
	if m != nil {
		a.Hosts = append([]string(nil), m.Access.Hosts...)
	}
	return a
}

// accessDiff lists what m asks for beyond a: "host:<name>", "event:<name>", "folder:<name>"
// (new, or read-write where read was approved) and "choose:<name>" for a
// required folder with no folder chosen. Empty means it may start.
func accessDiff(m *Manifest, a Approval) []string {
	if m == nil {
		return nil
	}
	var out []string
	hosts := map[string]bool{}
	for _, h := range a.Hosts {
		hosts[h] = true
	}
	for _, h := range m.Access.Hosts {
		if !hosts[h] {
			out = append(out, "host:"+h)
		}
	}
	if resourcesNeedOK(m.Resources, a.Resources) {
		out = append(out, "resources")
	}
	for _, sc := range m.Access.Actions {
		if !slices.Contains(a.Scopes, sc) {
			out = append(out, "scope:"+sc)
		}
	}
	for _, ev := range m.Access.Events {
		if !slices.Contains(a.Events, ev) {
			out = append(out, "event:"+ev)
		}
	}
	seen := map[string]FolderReq{}
	for _, f := range a.Folders {
		seen[f.Name] = f
	}
	for _, f := range m.Access.Folders {
		old, ok := seen[f.Name]
		switch {
		case !ok || (f.Mode == FolderReadWrite && old.Mode != FolderReadWrite):
			out = append(out, "folder:"+f.Name)
		case f.Required && a.Grants[f.Name].Path == "":
			out = append(out, "choose:"+f.Name)
		}
	}
	return out
}

// grantsFor are the folders a start gets: approved ones still asked for,
// with the lower of the asked and approved modes.
func grantsFor(m *Manifest, a Approval) []GrantedFolder {
	if m == nil {
		return nil
	}
	var out []GrantedFolder
	for _, f := range m.Access.Folders {
		g, ok := a.Grants[f.Name]
		if !ok || g.Path == "" {
			continue
		}
		mode := FolderRead
		if f.Mode == FolderReadWrite && g.Mode == FolderReadWrite {
			mode = FolderReadWrite
		}
		out = append(out, GrantedFolder{Name: f.Name, Path: g.Path, Mode: mode})
	}
	return out
}

// folderEnv names each folder for the plugin: HOLYARRS_PLUGIN_FOLDER_<NAME>
// and a JSON map in HOLYARRS_PLUGIN_FOLDERS.
func folderEnv(fs []GrantedFolder) []string {
	if len(fs) == 0 {
		return nil
	}
	m := map[string]string{}
	var out []string
	for _, f := range fs {
		m[f.Name] = f.Path
		key := strings.Map(func(r rune) rune {
			switch {
			case r >= 'a' && r <= 'z':
				return r - 32
			case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
				return r
			}
			return '_'
		}, f.Name)
		out = append(out, "HOLYARRS_PLUGIN_FOLDER_"+key+"="+f.Path)
	}
	raw, _ := json.Marshal(m)
	return append(out, "HOLYARRS_PLUGIN_FOLDERS="+string(raw))
}

// within reports whether p is base or below it.
func within(p, base string) bool {
	if base == "/" {
		return true
	}
	return p == base || strings.HasPrefix(p, base+"/")
}

// checkFolder resolves a folder the owner picked and refuses system folders,
// HolyArrs's own config, the app binary and any folder holding one of those.
func checkFolder(path string, protected, inside []string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%w: %s is not a full path", ErrFolderRefused, path)
	}
	real, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrFolderRefused, err)
	}
	real = filepath.ToSlash(real)
	fi, err := os.Stat(real)
	if err != nil || !fi.IsDir() {
		return "", fmt.Errorf("%w: %s is not a folder", ErrFolderRefused, path)
	}
	for _, p := range append(append([]string{}, protected...), inside...) {
		if p != "" && within(filepath.ToSlash(p), real) {
			return "", fmt.Errorf("%w: %s holds %s", ErrFolderRefused, real, p)
		}
	}
	for _, p := range inside {
		if p != "" && within(real, filepath.ToSlash(p)) {
			return "", fmt.Errorf("%w: %s is inside %s", ErrFolderRefused, real, p)
		}
	}
	return real, nil
}

// protectedPaths are the paths no folder may hold (protected) or sit in (inside).
func (e *Engine) protectedPaths() (protected, inside []string) {
	if exe, err := os.Executable(); err == nil {
		if r, err := filepath.EvalSymlinks(exe); err == nil {
			exe = r
		}
		protected = append(protected, exe)
	}
	cfg := filepath.Dir(e.Dir)
	if r, err := filepath.EvalSymlinks(cfg); err == nil {
		cfg = r
	}
	if r, err := filepath.EvalSymlinks(e.Dir); err == nil {
		inside = append(inside, r)
	}
	inside = append(inside, cfg, "/proc", "/sys", "/dev", "/etc", "/run", "/var/run", "/boot", "/sys/fs/cgroup")
	return protected, inside
}

// ApproveInput is the owner's answer: a real folder per folder name (""
// skips an optional one), and optionally the owner-added hosts.
type ApproveInput struct {
	Folders    map[string]string `json:"folders"`
	OwnerHosts *[]string         `json:"owner_hosts,omitempty"`
	// Limits are the owner's caps for this plugin; nil keeps the saved ones.
	Limits *Resources `json:"limits,omitempty"`
}

// Approve records the owner's OK for what the installed version asks for
// and starts it when it is on.
func (e *Engine) Approve(id string, in ApproveInput) error {
	if err := e.init(); err != nil {
		return err
	}
	e.opMu.Lock()
	defer e.opMu.Unlock()
	if err := e.approve(id, in, true); err != nil {
		return err
	}
	e.mu.Lock()
	delete(e.fails, id) // the held update's message is answered
	e.mu.Unlock()
	p, err := e.installed(id)
	if err != nil {
		return err
	}
	if e.Runner == nil {
		return nil
	}
	if !p.Enabled && p.DisabledReason != ErrNeedsApproval.Error() && !strings.HasPrefix(p.DisabledReason, ErrFolderMoved.Error()) {
		return nil
	}
	_ = e.stopProc(id)
	if err := e.store.SetEnabled(id, true, ""); err != nil {
		return err
	}
	return e.start(id)
}

// approve checks and stores the approval; withHosts also approves the
// installed version's hosts. Called with opMu held.
func (e *Engine) approve(id string, in ApproveInput, withHosts bool) error {
	if err := e.promotePending(id); err != nil {
		return err
	}
	p, err := e.installed(id)
	if err != nil {
		return err
	}
	m := e.readManifest(id)
	if m == nil || m.Version != p.Version {
		return fmt.Errorf("%w: no manifest for %s", ErrNotInstalled, id)
	}
	a := approvalFor(p, m)
	if withHosts {
		a.Hosts = append([]string(nil), m.Access.Hosts...)
		a.Events = append([]string(nil), m.Access.Events...)
	}
	a.Folders = append([]FolderReq(nil), m.Access.Folders...)
	a.Scopes = append([]string(nil), m.Access.Actions...)
	a.Resources = m.Resources
	if in.Limits != nil {
		if _, err := limitsFor(m.Resources, in.Limits, readHostTotals()); err != nil {
			return err
		}
		a.OwnerLimits = in.Limits
	}
	sort.Strings(a.Scopes)
	grants := map[string]FolderGrant{}
	protected, inside := e.protectedPaths()
	for _, f := range m.Access.Folders {
		path, given := in.Folders[f.Name]
		if !given {
			if g, ok := a.Grants[f.Name]; ok {
				g.Mode = f.Mode
				grants[f.Name] = g
			}
			continue
		}
		if strings.TrimSpace(path) == "" {
			continue
		}
		real, err := checkFolder(path, protected, inside)
		if err != nil {
			return err
		}
		dev, ino, err := folderID(real)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrFolderRefused, err)
		}
		grants[f.Name] = FolderGrant{Path: real, Mode: f.Mode, Dev: dev, Ino: ino, ApprovedAt: time.Now().UTC()}
	}
	for name := range in.Folders {
		found := false
		for _, f := range m.Access.Folders {
			found = found || f.Name == name
		}
		if !found {
			return fmt.Errorf("%w: %s does not ask for a folder named %q", ErrFolderRefused, id, name)
		}
	}
	a.Grants = grants
	if in.OwnerHosts != nil {
		hosts, err := cleanOwnerHosts(*in.OwnerHosts)
		if err != nil {
			return err
		}
		a.OwnerHosts = hosts
	}
	for _, f := range m.Access.Folders {
		if f.Required && a.Grants[f.Name].Path == "" {
			return fmt.Errorf("%w: choose a folder for %s", ErrFolderRefused, f.Name)
		}
	}
	return e.store.SetApproval(id, &a)
}

// cleanOwnerHosts checks and sorts the owner-added hosts.
func cleanOwnerHosts(in []string) ([]string, error) {
	if len(in) > maxOwnerHosts {
		return nil, fmt.Errorf("%w: up to %d hosts of your own", ErrSettings, maxOwnerHosts)
	}
	var out []string
	seen := map[string]bool{}
	for _, h := range in {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		if u, err := url.Parse(h); err == nil && u.Host != "" {
			h = u.Host
		}
		n, port, ok := ParseOwnerHost(h)
		if !ok {
			return nil, fmt.Errorf("%w: %q is not a host name (an address like 192.168.1.5 is not allowed, use a name)", ErrSettings, h)
		}
		if port != 0 {
			n = fmt.Sprintf("%s:%d", n, port)
		}
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out, nil
}

// SetOwnerHosts saves the owner-added hosts of id and restarts it when it runs.
func (e *Engine) SetOwnerHosts(id string, hosts []string) error {
	if err := e.init(); err != nil {
		return err
	}
	clean, err := cleanOwnerHosts(hosts)
	if err != nil {
		return err
	}
	e.opMu.Lock()
	defer e.opMu.Unlock()
	p, err := e.installed(id)
	if err != nil {
		return err
	}
	a := approvalFor(p, e.readManifest(id))
	a.OwnerHosts = clean
	if err := e.store.SetApproval(id, &a); err != nil {
		return err
	}
	e.mu.Lock()
	_, running := e.procs[id]
	e.mu.Unlock()
	if !running || e.Runner == nil {
		return nil
	}
	if err := e.stopProc(id); err != nil {
		return err
	}
	return e.start(id)
}

// openGrants opens each approved folder and checks it is still the one the
// owner chose. The caller closes the files.
func openGrants(fs []GrantedFolder, a Approval) ([]GrantedFolder, error) {
	out := make([]GrantedFolder, 0, len(fs))
	closeAll := func() {
		for _, f := range out {
			f.File.Close()
		}
	}
	for _, f := range fs {
		g := a.Grants[f.Name]
		file, err := openFolder(f.Path, g.Dev, g.Ino)
		if err != nil {
			closeAll()
			return nil, fmt.Errorf("%w (%s: %v)", ErrFolderMoved, f.Name, err)
		}
		f.File = file
		out = append(out, f)
	}
	return out, nil
}

// proxy is the engine's proxy, made on first use.
func (e *Engine) proxy() *Proxy {
	e.proxyOnce.Do(func() {
		if e.Proxy == nil {
			e.Proxy = &Proxy{}
		}
	})
	return e.Proxy
}

// selfAddr is the app's own address handed to the plugin in env
// (HOLYARRS_PLUGIN_TARGET), as host:port; "" when it has none.
func selfAddr(env []string) string {
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, "HOLYARRS_PLUGIN_TARGET="); ok {
			if u, err := url.Parse(v); err == nil && u.Port() != "" {
				return u.Host
			}
		}
	}
	return ""
}

// startAccess checks the approval and opens what one start of id gets: its
// proxy registration and its folders. Called with e.mu held.
func (e *Engine) startAccess(id string, env []string) (StartAccess, error) {
	p, err := e.installed(id)
	if err != nil {
		return StartAccess{}, err
	}
	m := e.readManifest(id)
	if m != nil && m.Version != p.Version {
		m = nil
	}
	a := approvalFor(p, m)
	if diff := accessDiff(m, a); len(diff) > 0 {
		return StartAccess{}, ErrNeedsApproval
	}
	acc := StartAccess{SelfAddr: selfAddr(env), Env: []string{"HOLYARRS_PLUGIN_NO_UDP=1"}}
	var asked *Resources
	if m != nil {
		asked = m.Resources
	}
	if acc.Limits, err = limitsFor(asked, a.OwnerLimits, readHostTotals()); err != nil {
		return StartAccess{}, err
	}
	folders, err := openGrants(grantsFor(m, a), a)
	if err != nil {
		return StartAccess{}, err
	}
	acc.Folders = folders
	acc.Events = eventsFor(m, a)
	acc.Env = append(acc.Env, folderEnv(folders)...)
	var hosts []string
	if m != nil {
		hosts = m.Access.Hosts
	}
	if len(hosts) > 0 || len(a.OwnerHosts) > 0 {
		h, err := e.proxy().Register(id, hosts, a.OwnerHosts)
		if err != nil {
			for _, f := range folders {
				f.File.Close()
			}
			return StartAccess{}, err
		}
		acc.Proxy = h
		acc.Env = append(acc.Env, h.Env(acc.SelfAddr)...)
	}
	return acc, nil
}

// showAccess fills a row's NeedsApproval (what the installed version asks
// for beyond the owner's OK: "host:x", "folder:x", "choose:x"), its chosen
// folders, owner-added hosts and refused connections since it started.
func (e *Engine) showAccess(p *Plugin, in *Installed, m *Manifest) {
	if pm := e.pendingManifest(in.ID, in); pm != nil {
		m = pm // what the waiting update asks for; the running version keeps its own
		p.NeedsApproval = accessDiff(pm, approvalFor(in, pm))
		a := approvalFor(in, m)
		p.Grants, p.OwnerHosts, p.Resources, p.OwnerLimits = a.Grants, a.OwnerHosts, pm.Resources, a.OwnerLimits
	} else if m != nil && m.Version == in.Version {
		a := approvalFor(in, m)
		p.NeedsApproval, p.Grants, p.OwnerHosts = accessDiff(m, a), a.Grants, a.OwnerHosts
		p.Resources, p.OwnerLimits = m.Resources, a.OwnerLimits
	}
	p.BlockedCount, p.BlockedRecent = e.proxy().Blocked(p.ID)
}

// keepApproval runs before an install: an update of a plugin from before
// 1.2.1 first stores what the owner saw then, so an update that adds hosts
// is still held. It reports whether this is a first install.
func (e *Engine) keepApproval(id string) (fresh bool, err error) {
	old, _ := e.installed(id)
	if old == nil {
		return true, nil
	}
	if old.Approval == nil {
		a := approvalFor(old, e.readManifest(id))
		return false, e.store.SetApproval(id, &a)
	}
	return false, nil
}

// firstApproval approves a first install: the hosts the owner saw and the
// folders in ap. Without a required folder the plugin waits for approval.
func (e *Engine) firstApproval(id string, ap *ApproveInput) error {
	in := ApproveInput{}
	if ap != nil {
		in = *ap
	}
	if err := e.approve(id, in, true); err != nil {
		_ = e.store.SetEnabled(id, false, ErrNeedsApproval.Error())
		return err
	}
	return nil
}

// GrantedScopes are the API scopes a start of id gets: what the installed
// version asks for, when the owner approved all of it; none otherwise.
func (e *Engine) GrantedScopes(id string) []string {
	if e.init() != nil {
		return nil
	}
	p, err := e.installed(id)
	if err != nil {
		return nil
	}
	m := e.readManifest(id)
	if m == nil || m.Version != p.Version || len(accessDiff(m, approvalFor(p, m))) > 0 {
		return nil
	}
	out := slices.Clone(m.Access.Actions)
	if p.Approval != nil {
		for _, k := range p.Approval.Kept {
			if !slices.Contains(out, k) {
				out = append(out, k)
			}
		}
	}
	sort.Strings(out)
	return out
}

// GrantedFolders are the real folders the owner gave id, when its current
// approval covers what it asks for; none otherwise.
func (e *Engine) GrantedFolders(id string) []string {
	if len(e.GrantedScopes(id)) == 0 {
		return nil
	}
	p, err := e.installed(id)
	if err != nil || p.Approval == nil {
		return nil
	}
	var out []string
	for _, g := range p.Approval.Grants {
		out = append(out, g.Path)
	}
	sort.Strings(out)
	return out
}
