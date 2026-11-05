// Package doctor checks the things people usually open a shell for: who
// HolyArrs runs as, whether its folders can be written, free space,
// hardlinks, the metadata server and the clock. Each finding is one line
// marked OK, WARN or FAIL, with a one-line fix in plain words. It never
// prints secrets: only paths, ids, sizes and the metadata server's host.
package doctor

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs/internal/importer"
	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/runas"
	"github.com/CoderLuii/holyarrs/internal/source"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// Status marks one finding.
type Status string

const (
	OK   Status = "OK"
	Warn Status = "WARN"
	Fail Status = "FAIL"
)

// MinYear is the earliest year a correct clock can show: HolyArrs was
// built after it, so an earlier date means the clock is wrong.
const MinYear = 2026

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
		if c.Status == Fail {
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
	fmt.Fprintf(&b, "\nResult: %d OK, %d WARN, %d FAIL\n", n[OK], n[Warn], n[Fail])
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
type Options struct {
	Version   string
	ConfigDir string
	// DB is the main database to read the folders from. Nil opens it
	// read-only from ConfigDir (and skips the Library folders when there
	// is none yet).
	DB       store.Lister
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
func Run(ctx context.Context, o Options) Report {
	var r Report
	uid, gid := os.Geteuid(), os.Getegid()
	appChecks(&r, o.Version, uid, gid)
	configChecks(&r, o.ConfigDir, uid, gid, store.NetworkShare)
	db := o.DB
	if db == nil && o.ConfigDir != "" {
		if d, err := store.OpenReadOnly(o.ConfigDir); err == nil {
			defer d.Close()
			db = d
		} else if errors.Is(err, fs.ErrNotExist) {
			r.add("Config folder", Warn, "no database yet, so no Library folders to check", "start HolyArrs once with this config folder")
		} else {
			// Usually permissions: the database or its -shm/-wal files
			// cannot be opened by this user.
			r.add("Config folder", Fail, "cannot open the database read-only, so no Library folders to check: "+err.Error(),
				writeFix(o.ConfigDir, uid, gid, configOwner(o.ConfigDir)))
		}
	}
	budget := o.FolderBudget
	if budget <= 0 {
		budget = doctorFolderBudget
	}
	folders, err := CheckFolders(ctx, db, o.SavePath, budget)
	if err != nil {
		r.add("Library and download folders", Warn, "could not read the folders from the database: "+err.Error(), "")
	}
	if len(folders) == 0 && err == nil {
		r.add("Library and download folders", Warn, "no Library or download folders set yet", "add a Library folder in Settings")
	}
	for _, f := range folders {
		c := f.Check(uid, gid)
		r.add("Library and download folders", c.Status, c.Text, c.Fix)
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
	client := o.HTTP
	if client == nil {
		client = metadataClient()
	}
	networkChecks(ctx, &r, raw, timeout, client, net.DefaultResolver.LookupHost)
	if w := ProxyWarning(os.Getenv); w != "" {
		r.add("Network", Warn, w, proxyFix)
	}
	now := o.Now
	if now.IsZero() {
		now = time.Now()
	}
	clockChecks(&r, now, os.Getenv("TZ"))
	return r
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
	allow := source.PublicOnly
	if on, _ := strconv.ParseBool(os.Getenv("HOLYARRS_METADATA_SERVICE_ALLOW_LOCAL")); on {
		allow = source.MetadataServiceLocal
	}
	return &http.Client{
		Transport:     metadata.NewServerTransport(source.PolicyDialer(allow).DialContext),
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
	r.add(s, OK, "version "+version+" ("+runtime.GOOS+"/"+runtime.GOARCH+")", "")
	if uid < 0 {
		r.add(s, OK, "running as the current Windows user", "")
	} else {
		groups := "none"
		if gs, err := os.Getgroups(); err == nil && len(gs) > 0 {
			var parts []string
			for _, g := range gs {
				parts = append(parts, strconv.Itoa(g))
			}
			groups = strings.Join(parts, ",")
		}
		st, fix := OK, ""
		if uid == 0 {
			st, fix = Warn, "running as root: set PUID and PGID to the owner of your media folders"
		}
		r.add(s, st, "running as "+ids(uid, gid)+", groups "+groups+", umask "+umask(), fix)
	}
	inside := runas.InContainer(func(p string) bool { _, err := os.Stat(p); return err == nil }, os.Getenv)
	r.add(s, OK, "in a container: "+yesNo(inside), "")
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

// writeFix is the plain fix for a folder this user cannot write to.
func writeFix(path string, uid, gid int, owner string) string {
	if uid < 0 {
		return "give the current user write access to " + path
	}
	fix := fmt.Sprintf("not writable by %s: run chown -R %s on the host folder mounted at %s, or set PUID/PGID to the folder's owner", ids(uid, gid), ids(uid, gid), path)
	if owner != "" {
		fix += " (" + owner + ")"
	}
	return fix
}

func configChecks(r *Report, dir string, uid, gid int, share func(string) (bool, string, error)) {
	const s = "Config folder"
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		r.add(s, Fail, dir+": the folder does not exist", "mount a local folder at "+dir+" (the /config volume)")
		return
	}
	owner, mode := ownerOf(st), st.Mode().String()
	text := dir + ": mode " + mode
	if owner != "" {
		text += ", owner " + owner
	}
	r.add(s, OK, text, "")
	if writable(dir) {
		r.add(s, OK, "writable", "")
	} else {
		r.add(s, Fail, "not writable", writeFix(dir, uid, gid, owner))
	}
	if free := importer.FreeSpace(dir); free >= 0 {
		if free < library.MinFreeSpace {
			r.add(s, Fail, "free space "+Size(free), "free up space on the disk that holds "+dir)
		} else {
			r.add(s, OK, "free space "+Size(free), "")
		}
	}
	if on, kind, err := share(dir); err != nil {
		r.add(s, Warn, "could not tell whether it is a local disk: "+err.Error(), "")
	} else if on {
		r.add(s, Fail, "on a network share ("+kind+"); the database must be on a local disk",
			"move the config folder to a local disk (for example appdata on a cache drive), not NFS or SMB")
	} else {
		r.add(s, OK, "on a local disk (the database must be local)", "")
	}
}

// writable creates and removes a temporary file in dir.
func writable(dir string) bool {
	f, err := os.CreateTemp(dir, ".holyarrs-doctor-*")
	if err != nil {
		return false
	}
	_ = f.Close()
	_ = os.Remove(f.Name())
	return true
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
		r.add(s, Fail, "the metadata server address is not a valid link", "fix "+metadata.EnvServiceURL+" or leave it unset")
		return
	}
	host := u.Hostname()
	dctx, cancel := context.WithTimeout(ctx, timeout)
	addrs, err := lookup(dctx, host)
	cancel()
	if err != nil {
		r.add(s, Fail, "DNS: cannot resolve "+host, "check the container's DNS (the dns: setting, a VPN or Pi-hole in front of it)")
	} else {
		if len(addrs) > 4 {
			addrs = addrs[:4]
		}
		r.add(s, OK, "DNS: "+host+" is "+strings.Join(addrs, ", "), "")
	}
	target := (&url.URL{Scheme: u.Scheme, Host: u.Host, Path: "/"}).String()
	hctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(hctx, http.MethodGet, target, nil)
	if err != nil {
		r.add(s, Fail, "cannot make a request to "+host, "")
		return
	}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		r.add(s, Fail, "metadata server "+host+" could not be reached (waited up to "+timeout.String()+")",
			"check the container's internet access (firewall, VPN container, proxy) and that the clock is right")
		return
	}
	_ = resp.Body.Close()
	r.add(s, OK, fmt.Sprintf("metadata server %s answered (HTTP %d) in %d ms", host, resp.StatusCode, time.Since(start).Milliseconds()), "")
}

func clockChecks(r *Report, now time.Time, tz string) {
	const s = "Clock"
	if tz == "" {
		zone, _ := now.Zone()
		tz = "not set, so " + zone
	}
	r.add(s, OK, "time zone (TZ): "+tz, "")
	text := "now " + now.Format("2006-01-02 15:04:05 MST")
	if now.Year() < MinYear {
		r.add(s, Warn, text+", which is before HolyArrs was built", "set the host's clock right (turn on NTP); a wrong clock breaks HTTPS and schedules")
		return
	}
	r.add(s, OK, text, "")
}

// readFolders lists the Library folders (path, kind).
func readFolders(ctx context.Context, db store.Lister) ([][2]string, error) {
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
