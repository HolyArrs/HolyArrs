package api

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/CoderLuii/holyarrs/internal/applog"
	"github.com/CoderLuii/holyarrs/internal/auth"
	"github.com/CoderLuii/holyarrs/internal/health"
	"github.com/CoderLuii/holyarrs/internal/importer"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// maxLogDownload caps the log download (the newest whole lines).
const maxLogDownload = 2 << 20

func (a *API) thisAppRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/app/storage", a.storage)
	m.HandleFunc("GET /api/v1/app/about", a.about)
	m.HandleFunc("GET /api/v1/logs", a.readLogs)
	m.HandleFunc("GET /api/v1/logs/download", a.downloadLogs)
	m.HandleFunc("GET /api/v1/app/update", a.getUpdate)
	m.HandleFunc("POST /api/v1/app/update/check", a.checkUpdate)
}

// getUpdate answers the update check's state; it never asks the feed.
func (a *API) getUpdate(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.updateView(r, a.s.Health.Update(r.Context())))
}

// updateView shows the feed only to people who may change settings, and
// then as scheme, host and path only (never a name, password or query).
func (a *API) updateView(r *http.Request, u health.UpdateInfo) health.UpdateInfo {
	if who, ok := auth.From(r.Context()); ok && !who.Has(auth.PermSettings) {
		u.Feed = ""
		return u
	}
	if p, err := url.Parse(u.Feed); err == nil {
		u.Feed = (&url.URL{Scheme: p.Scheme, Host: p.Host, Path: p.Path}).String()
	} else {
		u.Feed = ""
	}
	return u
}

// checkUpdate asks the release feed now (nothing when the check is off).
// HolyArrs never updates itself.
func (a *API) checkUpdate(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.updateView(r, a.s.Health.CheckUpdate(r.Context())))
}

// PathInfo is one folder or file as HolyArrs sees it.
type PathInfo struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Bytes int64  `json:"bytes,omitempty"`
	// Free is the free space on its disk (-1 unknown).
	Free int64  `json:"free_bytes"`
	Kind string `json:"kind,omitempty"`
}

func fileSize(p string) int64 {
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		return st.Size()
	}
	return 0
}

func (a *API) configDir() string {
	if a.s.ConfigDir != "" {
		return a.s.ConfigDir
	}
	return a.s.Store.Dir()
}

// storage lists where HolyArrs keeps things, as the app sees the paths
// (inside the container), with free space.
func (a *API) storage(w http.ResponseWriter, r *http.Request) {
	dir := a.configDir()
	free := importer.FreeSpace(dir)
	out := []PathInfo{
		{Name: "Config folder", Path: dir, Free: free},
		{Name: "Main database", Path: filepath.Join(dir, store.MainFile), Bytes: fileSize(filepath.Join(dir, store.MainFile)), Free: free},
		{Name: "History database", Path: filepath.Join(dir, store.EventsFile), Bytes: fileSize(filepath.Join(dir, store.EventsFile)), Free: free},
		{Name: "Backups", Path: filepath.Join(dir, "backups"), Free: free},
		{Name: "Artwork", Path: filepath.Join(dir, "artwork"), Free: free},
	}
	if a.s.Log != nil {
		out = append(out, PathInfo{Name: "Log files", Path: a.s.Log.Dir(), Bytes: fileSize(filepath.Join(a.s.Log.Dir(), applog.FileName)) +
			fileSize(filepath.Join(a.s.Log.Dir(), applog.OldName)), Free: free})
	}
	media := []PathInfo{}
	rows, err := a.s.Store.Read().QueryContext(r.Context(), "SELECT path, kind, label FROM library_folders ORDER BY kind, path")
	if err != nil {
		internal(w, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var p PathInfo
		if err := rows.Scan(&p.Path, &p.Kind, &p.Name); err != nil {
			internal(w, err)
			return
		}
		p.Free = importer.FreeSpace(p.Path)
		media = append(media, p)
	}
	if err := rows.Err(); err != nil {
		internal(w, err)
		return
	}
	save := a.s.SavePath
	writeJSON(w, http.StatusOK, map[string]any{"app": out, "library_folders": media, "save_path": save})
}

// logsReady brings the masked values up to date (old values of changed
// or removed secrets are masked in the files first) and answers whether the log
// is kept here.
func (a *API) logsReady(w http.ResponseWriter, r *http.Request) bool {
	if a.s.Log == nil {
		fail(w, http.StatusServiceUnavailable, "this HolyArrs keeps no log file; read the program's output (docker logs)")
		return false
	}
	if a.s.Secrets != nil {
		if err := a.s.Log.Sync(r.Context(), a.s.Secrets); err != nil {
			internal(w, err)
			return false
		}
	}
	return true
}

// readLogs is the log viewer: ?level=info|warning|error (least serious
// shown), q=text, from=, to= (a date or an RFC 3339 time), offset, limit.
func (a *API) readLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := applog.Filter{Level: q.Get("level"), Text: q.Get("q"), Limit: limit(r, 100)}
	switch f.Level {
	case "", applog.Info, applog.Warning, applog.Error:
	default:
		fail(w, http.StatusBadRequest, "level is info, warning or error")
		return
	}
	if len(f.Text) > 200 {
		fail(w, http.StatusBadRequest, "q is at most 200 characters")
		return
	}
	if v := q.Get("from"); v != "" {
		t, err := parseDay(v)
		if err != nil {
			fail(w, http.StatusBadRequest, "from must be a date like 2026-10-01 or a full time")
			return
		}
		f.From = t
	}
	if v := q.Get("to"); v != "" {
		t, err := parseDay(v)
		if err != nil {
			fail(w, http.StatusBadRequest, "to must be a date like 2026-10-31 or a full time")
			return
		}
		if len(v) == len("2006-01-02") {
			t = t.AddDate(0, 0, 1)
		}
		f.To = t
	}
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			fail(w, http.StatusBadRequest, "offset must be 0 or more")
			return
		}
		f.Offset = n
	}
	if !a.logsReady(w, r) {
		return
	}
	lines, total := a.s.Log.Read(f)
	if lines == nil {
		lines = []applog.Line{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"lines": lines, "total": total, "offset": f.Offset, "limit": f.Limit})
}

// downloadLogs hands out the newest log text (at most 2 MB), masked again.
func (a *API) downloadLogs(w http.ResponseWriter, r *http.Request) {
	if !a.logsReady(w, r) {
		return
	}
	b := a.s.Log.Tail(maxLogDownload)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="holyarrs-log-`+time.Now().UTC().Format("20060102-150405")+`.txt"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(b)
}

// AboutView is what the Status page's About section shows, like the About
// block of the apps HolyArrs replaces: versions, database, folders, uptime.
type AboutView struct {
	Version   string `json:"version"`
	Go        string `json:"go"`
	OS        string `json:"os"`
	Docker    bool   `json:"docker"`
	SQLite    string `json:"sqlite"`
	Migration int    `json:"migration"`
	ConfigDir string `json:"config_dir"`
	StartDir  string `json:"start_dir"`
	UptimeS   int64  `json:"uptime_s"`
	Started   string `json:"started_at"`
}

func (a *API) about(w http.ResponseWriter, r *http.Request) {
	v := AboutView{
		Version: a.s.Version, Go: runtime.Version(), OS: runtime.GOOS + "/" + runtime.GOARCH,
		ConfigDir: a.configDir(), UptimeS: int64(time.Since(a.s.Started).Seconds()), Started: a.s.Started.UTC().Format(time.RFC3339),
	}
	if _, err := os.Stat("/.dockerenv"); err == nil {
		v.Docker = true
	}
	if exe, err := os.Executable(); err == nil {
		v.StartDir = filepath.Dir(exe)
	}
	_ = a.s.Store.Read().QueryRowContext(r.Context(), "SELECT sqlite_version()").Scan(&v.SQLite)
	v.Migration, _ = a.s.Store.Version()
	writeJSON(w, http.StatusOK, v)
}
