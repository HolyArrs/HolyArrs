package health

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/files"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
)

// Status marks one finding.
type Status string

const (
	StatusOK   Status = "OK"
	StatusWarn Status = "WARN"
	StatusFail Status = "FAIL"
)

// MinYear is the earliest year a correct clock can show: HolyArrs was
// built after it, so an earlier date means the clock is wrong.
const ClockMinYear = 2026

// doctorFolderBudget is the default time limit for the folder checks in
// holyarrs doctor, longer than the 3 seconds at start-up.
const doctorFolderBudget = 20 * time.Second

// Check is one line of the report.
type Check struct {
	Section string `json:"section"`
	Status  Status `json:"status"`
	Text    string `json:"text"`
	Fix     string `json:"fix,omitempty"`
}

// Report is every finding, in order.
type Report struct {
	Checks []Check `json:"checks"`
}

func (r *Report) add(section string, st Status, text, fix string) {
	r.Checks = append(r.Checks, Check{Section: section, Status: st, Text: text, Fix: fix})
}

// Failed reports whether any finding is FAIL.
func (r Report) Failed() bool {
	for _, c := range r.Checks {
		if c.Status == StatusFail {
			return true
		}
	}
	return false
}

// Text is the plain-text report: short sections, one line per finding,
// the fix under it.
func (r Report) Text() string {
	var b strings.Builder
	b.WriteString("HolyArrs doctor\n")
	section := ""
	n := map[Status]int{}
	for _, c := range r.Checks {
		if c.Section != section {
			section = c.Section
			fmt.Fprintf(&b, "\n%s\n", section)
		}
		n[c.Status]++
		fmt.Fprintf(&b, "  %-4s  %s\n", c.Status, c.Text)
		if c.Fix != "" {
			fmt.Fprintf(&b, "        fix: %s\n", c.Fix)
		}
	}
	fmt.Fprintf(&b, "\nResult: %d OK, %d WARN, %d FAIL\n", n[StatusOK], n[StatusWarn], n[StatusFail])
	return b.String()
}

// JSON is the report as JSON, with ok false when any finding is FAIL.
func (r Report) JSON() []byte {
	b, _ := json.MarshalIndent(struct {
		OK     bool    `json:"ok"`
		Checks []Check `json:"checks"`
	}{!r.Failed(), r.Checks}, "", "  ")
	return append(b, '\n')
}

// Options are what Run checks. Zero values pick the normal setup.
type DoctorOptions struct {
	Version   string
	ConfigDir string
	// DB is the main database to read the folders from. Nil opens it
	// read-only from ConfigDir (and skips the Library folders when there
	// is none yet).
	DB       database.Lister
	SavePath string
	// MetadataURL is the metadata server; empty means HOLYARRS_METADATA_URL
	// or the default server.
	MetadataURL string
	// FolderBudget caps the folder checks together (default
	// doctorFolderBudget).
	FolderBudget time.Duration
	// NetTimeout caps the DNS lookup and the request (default 5s each).
	NetTimeout time.Duration
	// HTTP makes the request (default: a plain client).
	HTTP *http.Client
	// Now is the current time (default time.Now()).
	Now time.Time
}

// Run makes the whole report.
func RunDoctor(ctx context.Context, o DoctorOptions) Report {
	var r Report
	uid, gid := os.Geteuid(), os.Getegid()
	appChecks(&r, o.Version, uid, gid)
	configChecks(&r, o.ConfigDir, uid, gid, files.IsNetworkShare)
	if o.ConfigDir != "" {
		if lockProbe != nil {
			lockChecks(&r, o.ConfigDir, lockProbe)
		}
		unmappedChecks(&r, o.ConfigDir, unmappedConfig(o.ConfigDir))
	}
	masterKeyChecks(&r, o.ConfigDir)
	db := o.DB
	skipFolders := false
	if db == nil && o.ConfigDir != "" {
		if d, err := database.OpenReadOnly(o.ConfigDir); err == nil {
			defer d.Close()
			db = d
			skipFolders = schemaChecks(&r, d)
		} else if errors.Is(err, fs.ErrNotExist) {
			r.add("Config folder", StatusWarn, "no database yet, so no Library folders to check", "start HolyArrs once with this config folder")
		} else {
			// Usually permissions: the database or its -shm/-wal files
			// cannot be opened by this user.
			r.add("Config folder", StatusFail, "cannot open the database read-only, so no Library folders to check: "+err.Error(),
				writeFix(o.ConfigDir, uid, gid, configOwner(o.ConfigDir), err))
		}
	}
	budget := o.FolderBudget
	if budget <= 0 {
		budget = doctorFolderBudget
	}
	if !skipFolders {
		folders, err := CheckFolders(ctx, db, o.SavePath, budget)
		if err != nil {
			r.add("Library and download folders", StatusWarn, "could not read the folders from the database: "+err.Error(), "")
		}
		if len(folders) == 0 && err == nil {
			r.add("Library and download folders", StatusWarn, "no Library or download folders set yet", "add a Library folder in Settings")
		}
		for _, f := range folders {
			c := f.Check(uid, gid)
			r.add("Library and download folders", c.Status, c.Text, c.Fix)
		}
		for _, w := range UnsafeFolderWarnings(ctx, db, o.ConfigDir) {
			r.add("Library and download folders", StatusWarn, w, "")
		}
	}
	raw := o.MetadataURL
	if raw == "" {
		raw = os.Getenv(metadata.EnvServiceURL)
	}
	if raw == "" {
		raw = metadata.DefaultServiceURL
	}
	timeout := o.NetTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	if !metadataModeChecks(ctx, &r, db) {
		client := o.HTTP
		if client == nil {
			client = metadataClient()
		}
		networkChecks(ctx, &r, raw, timeout, client, net.DefaultResolver.LookupHost)
	}
	if w := ProxyWarning(os.Getenv); w != "" {
		r.add("Network", StatusWarn, w, proxyFix)
	}
	now := o.Now
	if now.IsZero() {
		now = time.Now()
	}
	clockChecks(&r, now, os.Getenv("TZ"))
	return r
}

// schemaChecks compares the database's schema with this build's. While the
// server still sets up or updates the data (or the data is from a newer
// HolyArrs) the folders cannot be read yet: it says so in one WARN and
// returns true, so the folder checks are skipped.
func schemaChecks(r *Report, db *sql.DB) bool {
	know, _ := database.SchemaVersions()
	have, err := database.SchemaVersion(db)
	if err != nil {
		have = 0 // no schema_migrations table yet: setup has barely started
	}
	switch {
	case have < know:
		r.add("Config folder", StatusWarn, "the database is still being set up or updated, so Library folders were not checked",
			"if HolyArrs is starting, run doctor again in a few seconds; if it is stopped, start it so the update can finish")
		return true
	case have > know:
		r.add("Config folder", StatusWarn, (&database.NewerDataError{Have: have, Know: know}).Error(),
			"run the newer HolyArrs again, or restore a backup made by this version")
		return true
	}
	return false
}

// proxyNames are the usual environment proxy settings. HolyArrs ignores them
// on purpose: every outgoing connection checks the real address first, and an
// environment proxy would skip that check.
var proxyNames = []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"}

const proxyFix = "proxies for indexers can be set on each Source; HolyArrs has no app-wide proxy yet"

// ProxyWarning returns one line when a proxy setting is present, naming the
// settings found (never their values, which may hold a password), or "".
func ProxyWarning(getenv func(string) string) string {
	var found []string
	seen := map[string]bool{}
	for _, n := range proxyNames {
		if strings.TrimSpace(getenv(n)) != "" && !seen[strings.ToUpper(n)] {
			seen[strings.ToUpper(n)] = true
			found = append(found, n)
		}
	}
	if len(found) == 0 {
		return ""
	}
	return strings.Join(found, ", ") + " is set, but HolyArrs does not use it: posters, metadata and update checks connect directly and may fail"
}

// metadataClient reaches the metadata server the way the app does: no
// proxy from the environment, and the same address policy on every dial.
func metadataClient() *http.Client {
	allow := outbound.PublicOnly
	if on, _ := strconv.ParseBool(os.Getenv("HOLYARRS_METADATA_SERVICE_ALLOW_LOCAL")); on {
		allow = outbound.MetadataServiceLocal
	}
	return &http.Client{
		Transport:     metadata.NewServerTransport(outbound.PolicyDialer(allow).DialContext),
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// configOwner is the config folder's owner, or "" when unknown.
func configOwner(dir string) string {
	if st, err := os.Stat(dir); err == nil {
		return ownerOf(st)
	}
	return ""
}

func ids(uid, gid int) string { return strconv.Itoa(uid) + ":" + strconv.Itoa(gid) }

func appChecks(r *Report, version string, uid, gid int) {
	const s = "App"
	if version == "" {
		version = "unknown"
	}
	r.add(s, StatusOK, "version "+version+" ("+runtime.GOOS+"/"+runtime.GOARCH+")", "")
	if uid < 0 {
		r.add(s, StatusOK, "running as the current Windows user", "")
	} else {
		groups := "none"
		if gs, err := os.Getgroups(); err == nil && len(gs) > 0 {
			var parts []string
			for _, g := range gs {
				parts = append(parts, strconv.Itoa(g))
			}
			groups = strings.Join(parts, ",")
		}
		st, fix := StatusOK, ""
		if uid == 0 {
			st, fix = StatusWarn, "running as root: set PUID and PGID to the owner of your media folders (in rootless Docker or Podman, root in the container is your own user on the host, so this is expected there)"
		}
		r.add(s, st, "running as "+ids(uid, gid)+", groups "+groups+", umask "+umask(), fix)
	}
	inside := startup.InContainer(func(p string) bool { _, err := os.Stat(p); return err == nil }, os.Getenv)
	r.add(s, StatusOK, "in a container: "+yesNo(inside), "")
	if t := selinuxType(); t != "" {
		r.add(s, StatusOK, "SELinux: on ("+t+")", "")
	}
}

// selinuxType is this process's SELinux type ("container_t"), or "".
func selinuxType() string {
	b, err := os.ReadFile("/proc/self/attr/current")
	if err != nil {
		return ""
	}
	return files.SELinuxType(string(b))
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// umask reads the mask from /proc (Linux) without changing it.
func umask() string {
	b, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return "unknown"
	}
	return parseUmask(string(b))
}

func parseUmask(status string) string {
	for _, line := range strings.Split(status, "\n") {
		if v, ok := strings.CutPrefix(line, "Umask:"); ok {
			return strings.TrimSpace(v)
		}
	}
	return "unknown"
}

// writeFix is the plain fix for a folder this user cannot write to. err is
// why the write failed (nil when unknown): a read-only mount or a folder
// whose owner is already right does not get chown advice.
func writeFix(path string, uid, gid int, owner string, err error) string {
	if uid < 0 {
		return "give the current user write access to " + path
	}
	se := selinuxType() != ""
	switch {
	case errors.Is(err, syscall.EROFS):
		return "mounted read-only: remove :ro from the volume for " + path + " if HolyArrs should write there (changing the owner or PUID/PGID can't help a read-only mount)"
	case owner == ids(uid, gid) && (errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM)):
		fix := "the owner of " + path + " is already " + owner + ", so something else blocks writes: a security label (SELinux), an ACL, or the NAS's share permissions"
		if se {
			fix += "; with SELinux add :Z to the /config volume and :z to shared folders (never on system or home folders, and never turn SELinux off)"
		}
		return fix
	}
	fix := fmt.Sprintf("not writable by %s: run chown -R %s on the host folder mounted at %s, or set PUID/PGID to the folder's owner", ids(uid, gid), ids(uid, gid), path)
	if owner != "" {
		fix += " (" + owner + ")"
	}
	if se {
		fix += " (or SELinux labels: :Z on /config, :z on shared folders)"
	}
	return fix
}

func configChecks(r *Report, dir string, uid, gid int, share func(string) (bool, string, error)) {
	const s = "Config folder"
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		r.add(s, StatusFail, dir+": the folder does not exist", "mount a local folder at "+dir+" (the /config volume)")
		return
	}
	owner, mode := ownerOf(st), st.Mode().String()
	text := dir + ": mode " + mode
	if owner != "" {
		text += ", owner " + owner
	}
	r.add(s, StatusOK, text, "")
	if err := writable(dir); err == nil {
		r.add(s, StatusOK, "writable", "")
	} else {
		text := "not writable"
		if why := files.Why(err); why != "" {
			text += ": " + why
		}
		r.add(s, StatusFail, text, writeFix(dir, uid, gid, owner, err))
	}
	if free := importer.FreeSpace(dir); free >= 0 {
		if free < library.MinFreeSpace {
			r.add(s, StatusFail, "free space "+Size(free), "free up space on the disk that holds "+dir)
		} else {
			r.add(s, StatusOK, "free space "+Size(free), "")
		}
	}
	on, kind, err := share(dir)
	switch {
	case err != nil:
		r.add(s, StatusWarn, "could not tell whether it is a local disk: "+err.Error(), "")
	case !on:
		text := "on a local disk (the database must be local)"
		if fs := files.MountType(dir); fs != "" {
			text += ", mount type " + fs // for support: names the filesystem
		}
		r.add(s, StatusOK, text, "")
	case files.InMemory(kind):
		if files.ConfigInMemoryAllowed() {
			r.add(s, StatusWarn, "in memory ("+kind+"), kept there because "+files.EnvConfigInMemory+" is set; everything in it is lost at the next restart", "put /config on a disk and remove "+files.EnvConfigInMemory)
		} else {
			r.add(s, StatusFail, "in memory ("+kind+"); everything in it is lost at the next restart, so HolyArrs will not start",
				"put /config on a disk (on Unraid: /mnt/<pool>/appdata/holyarrs, and check that the pool exists)")
		}
	case kind == "fakeowner" && files.DesktopFolderAllowed():
		r.add(s, StatusWarn, "a Docker Desktop folder share, used because "+files.EnvAllowDesktopFolder+" is set; file locks there may not work", files.ShareFix(kind))
	default:
		text := "on a shared or network folder (" + kind + "); the database must be on a local disk"
		if fs := files.MountType(dir); fs != "" {
			if doctorLabel, _ := files.Family(fs); doctorLabel != "" {
				text = "on " + doctorLabel + " (" + fs + "); the database must be on a local disk"
			}
		}
		r.add(s, StatusFail, text, files.ShareFix(kind))
	}
}

// lockChecks reports whether file locks work in the config folder. It only
// reports: start-up never refuses on it.
func lockChecks(r *Report, dir string, probe func(string) error) {
	const s = "Config folder"
	if err := probe(dir); err != nil {
		r.add(s, StatusWarn, "file locks do not work in the config folder ("+err.Error()+"), so the database could be damaged",
			"use a named volume for /config (-v holyarrs-config:/config), or a local disk")
		return
	}
	r.add(s, StatusOK, "file locks work", "")
}

// unmappedChecks reports a /config that is not mapped to a folder on the
// host: kind is "anonymous" or "none" (UnmappedConfig); "" adds nothing.
func unmappedChecks(r *Report, dir, kind string) {
	if kind == "" {
		return
	}
	r.add("Config folder", StatusFail, UnmappedText(dir, kind), UnmappedFix(dir))
}

// UnmappedText says in one line that dir is not mapped, for doctor, the log
// and the health check.
func UnmappedText(dir, kind string) string {
	where := "an anonymous volume"
	if kind == "none" {
		where = "the container itself"
	}
	return dir + " is not mapped to a folder or named volume, so it lives in " + where + " and everything is lost when the container is removed or recreated"
}

// UnmappedFix is how to map it without losing what is there.
func UnmappedFix(dir string) string {
	return "stop the container first (docker stop <container>), then copy the data out with docker cp <container>:" + dir + " ./holyarrs-config (this works on a stopped container), then map " + dir + " to a host folder or a named volume (-v holyarrs-config:" + dir + ") and copy it back in"
}

// unmappedConfig is UnmappedConfig; tests swap it so a test run inside a
// container does not see its own temp folder as an unmapped /config.
var unmappedConfig = UnmappedConfig

// UnmappedConfig tells whether dir, inside a container, is not mapped to
// the host: "anonymous", "none", or "" (mapped, not in a container, or
// unsure).
func UnmappedConfig(dir string) string {
	exists := func(p string) bool { _, err := os.Stat(p); return err == nil }
	if !startup.InContainer(exists, os.Getenv) {
		return ""
	}
	b, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return ""
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
		if r, err := filepath.EvalSymlinks(abs); err == nil {
			dir = r
		}
	}
	return files.Unmapped(string(b), filepath.ToSlash(dir))
}

// writable creates and removes a temporary file in dir.
func writable(dir string) error {
	f, err := os.CreateTemp(dir, ".holyarrs-doctor-*")
	if err != nil {
		return err
	}
	_ = f.Close()
	_ = os.Remove(f.Name())
	return nil
}

// Size is a byte count in plain units.
func Size(n int64) string {
	const k = 1024
	switch {
	case n < 0:
		return "unknown"
	case n >= k*k*k*k:
		return fmt.Sprintf("%.1f TB", float64(n)/(k*k*k*k))
	case n >= k*k*k:
		return fmt.Sprintf("%.1f GB", float64(n)/(k*k*k))
	case n >= k*k:
		return fmt.Sprintf("%.0f MB", float64(n)/(k*k))
	}
	return fmt.Sprintf("%d bytes", n)
}

// networkChecks resolves the metadata server's host and makes one HTTPS
// request. Only the host is shown, never a path, user or query.
func networkChecks(ctx context.Context, r *Report, raw string, timeout time.Duration, client *http.Client, lookup func(context.Context, string) ([]string, error)) {
	const s = "Network"
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		r.add(s, StatusFail, "the metadata server address is not a valid link", "fix "+metadata.EnvServiceURL+" or leave it unset")
		return
	}
	host := u.Hostname()
	dctx, cancel := context.WithTimeout(ctx, timeout)
	addrs, err := lookup(dctx, host)
	cancel()
	if err != nil {
		r.add(s, StatusFail, "DNS: cannot resolve "+host, "check the container's DNS (the dns: setting, a VPN or Pi-hole in front of it)")
	} else {
		if len(addrs) > 4 {
			addrs = addrs[:4]
		}
		r.add(s, StatusOK, "DNS: "+host+" is "+strings.Join(addrs, ", "), "")
	}
	target := (&url.URL{Scheme: u.Scheme, Host: u.Host, Path: "/"}).String()
	hctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(hctx, http.MethodGet, target, nil)
	if err != nil {
		r.add(s, StatusFail, "cannot make a request to "+host, "")
		return
	}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		r.add(s, StatusFail, "metadata server "+host+" could not be reached (waited up to "+timeout.String()+")",
			"check the container's internet access (firewall, VPN container, proxy) and that the clock is right")
		return
	}
	_ = resp.Body.Close()
	r.add(s, StatusOK, fmt.Sprintf("metadata server %s answered (HTTP %d) in %d ms", host, resp.StatusCode, time.Since(start).Milliseconds()), "")
}

func clockChecks(r *Report, now time.Time, tz string) {
	const s = "Clock"
	if tz == "" {
		zone, _ := now.Zone()
		tz = "not set, so " + zone
	}
	r.add(s, StatusOK, "time zone (TZ): "+tz, "")
	text := "now " + now.Format("2006-01-02 15:04:05 MST")
	if now.Year() < ClockMinYear {
		r.add(s, StatusWarn, text+", which is before HolyArrs was built", "set the host's clock right (turn on NTP); a wrong clock breaks HTTPS and schedules")
		return
	}
	r.add(s, StatusOK, text, "")
}

// readFolders lists the Library folders (path, kind).
func readFolders(ctx context.Context, db database.Lister) ([][2]string, error) {
	rows, err := db.QueryContext(ctx, "SELECT path, kind FROM library_folders ORDER BY kind, path")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][2]string
	for rows.Next() {
		var p, k sql.NullString
		if err := rows.Scan(&p, &k); err != nil {
			return nil, err
		}
		out = append(out, [2]string{p.String, k.String})
	}
	return out, rows.Err()
}
