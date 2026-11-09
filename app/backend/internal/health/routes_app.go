package health

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/logs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
)

// maxLogDownload caps the log download (the newest whole lines).
const maxLogDownload = 2 << 20

// AppHandlers serves where HolyArrs keeps things, the About block, the log
// viewer and the support bundle.
type AppHandlers struct {
	Store   *database.Store
	Version string
	Started time.Time
	// Log is the app log (nil: none kept).
	Log *logs.Log
	// Secrets masks changed secrets in the log before it is shown (may be nil).
	Secrets *secrets.Vault
	// ConfigDir and SavePath are shown as they are; an empty ConfigDir
	// falls back to the database folder.
	ConfigDir string
	SavePath  string
	// HealthView is the app's own health answer, put in the support bundle (may be nil).
	HealthView func(ctx context.Context) any
}

// Routes adds the storage, About, log and support bundle routes.
func (h *AppHandlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/app/storage", h.storage)
	m.HandleFunc("GET /api/v1/app/about", h.about)
	m.HandleFunc("GET /api/v1/logs", h.readLogs)
	m.HandleFunc("GET /api/v1/logs/download", h.downloadLogs)
	m.HandleFunc("GET /api/v1/support/bundle", h.supportBundle)
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

func (h *AppHandlers) configDir() string {
	if h.ConfigDir != "" {
		return h.ConfigDir
	}
	return h.Store.Dir()
}

// storage lists where HolyArrs keeps things, as the app sees the paths
// (inside the container), with free space.
func (h *AppHandlers) storage(w http.ResponseWriter, r *http.Request) {
	dir := h.configDir()
	free := importer.FreeSpace(dir)
	out := []PathInfo{
		{Name: "Config folder", Path: dir, Free: free},
		{Name: "Main database", Path: filepath.Join(dir, database.MainFile), Bytes: fileSize(filepath.Join(dir, database.MainFile)), Free: free},
		{Name: "History database", Path: filepath.Join(dir, database.EventsFile), Bytes: fileSize(filepath.Join(dir, database.EventsFile)), Free: free},
		{Name: "Backups", Path: filepath.Join(dir, "backups"), Free: free},
		{Name: "Artwork", Path: filepath.Join(dir, "artwork"), Free: free},
	}
	if h.Log != nil {
		out = append(out, PathInfo{Name: "Log files", Path: h.Log.Dir(), Bytes: fileSize(filepath.Join(h.Log.Dir(), logs.FileName)) +
			fileSize(filepath.Join(h.Log.Dir(), logs.OldName)), Free: free})
	}
	media := []PathInfo{}
	folders, err := libraryFolders(r.Context(), h.Store.Read())
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	for _, d := range folders {
		media = append(media, PathInfo{Path: d.Path, Kind: d.Kind, Name: d.Label, Free: importer.FreeSpace(d.Path)})
	}
	save := h.SavePath
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"app": out, "library_folders": media, "save_path": save})
}

// logsReady brings the masked values up to date (old values of changed
// or removed secrets are masked in the files first) and answers whether the log
// is kept here.
func (h *AppHandlers) logsReady(w http.ResponseWriter, r *http.Request) bool {
	if h.Log == nil {
		webserver.Fail(w, http.StatusServiceUnavailable, "this HolyArrs keeps no log file; read the program's output (docker logs)")
		return false
	}
	if h.Secrets != nil {
		if err := h.Log.Sync(r.Context(), h.Secrets); err != nil {
			webserver.Internal(w, "api", err)
			return false
		}
	}
	return true
}

// readLogs is the log viewer: ?level=info|warning|error (least serious
// shown), q=text, from=, to= (a date or an RFC 3339 time), offset, limit.
func (h *AppHandlers) readLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := logs.Filter{Level: q.Get("level"), Text: q.Get("q"), Limit: queryLimit(r, 100)}
	switch f.Level {
	case "", logs.Info, logs.Warning, logs.Error:
	default:
		webserver.Fail(w, http.StatusBadRequest, "level is info, warning or error")
		return
	}
	if len(f.Text) > 200 {
		webserver.Fail(w, http.StatusBadRequest, "q is at most 200 characters")
		return
	}
	if v := q.Get("from"); v != "" {
		t, err := parseDay(v)
		if err != nil {
			webserver.Fail(w, http.StatusBadRequest, "from must be a date like 2026-10-01 or a full time")
			return
		}
		f.From = t
	}
	if v := q.Get("to"); v != "" {
		t, err := parseDay(v)
		if err != nil {
			webserver.Fail(w, http.StatusBadRequest, "to must be a date like 2026-10-31 or a full time")
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
			webserver.Fail(w, http.StatusBadRequest, "offset must be 0 or more")
			return
		}
		f.Offset = n
	}
	if !h.logsReady(w, r) {
		return
	}
	lines, total := h.Log.Read(f)
	if lines == nil {
		lines = []logs.Line{}
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"lines": lines, "total": total, "offset": f.Offset, "limit": f.Limit})
}

// downloadLogs hands out the newest log text (at most 2 MB), masked again.
func (h *AppHandlers) downloadLogs(w http.ResponseWriter, r *http.Request) {
	if !h.logsReady(w, r) {
		return
	}
	b := h.Log.Tail(maxLogDownload)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="holyarrs-log-`+time.Now().UTC().Format("20060102-150405")+`.txt"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(b)
}

// AboutView is what the Health page's About section shows, like the About
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

func (h *AppHandlers) about(w http.ResponseWriter, r *http.Request) {
	webserver.WriteJSON(w, http.StatusOK, h.aboutView(r.Context()))
}

func (h *AppHandlers) aboutView(ctx context.Context) AboutView {
	v := AboutView{
		Version: h.Version, Go: runtime.Version(), OS: runtime.GOOS + "/" + runtime.GOARCH,
		ConfigDir: h.configDir(), UptimeS: int64(time.Since(h.Started).Seconds()), Started: h.Started.UTC().Format(time.RFC3339),
	}
	// Docker, Podman, Kubernetes and the rest: the update steps are the same.
	v.Docker = startup.InContainer(func(p string) bool { _, err := os.Stat(p); return err == nil }, os.Getenv)
	if exe, err := os.Executable(); err == nil {
		v.StartDir = filepath.Dir(exe)
	}
	v.SQLite = sqliteVersion(ctx, h.Store.Read())
	v.Migration, _ = h.Store.Version()
	return v
}

// queryLimit reads ?limit= (1 to 500), else def.
func queryLimit(r *http.Request, def int) int {
	n, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || n <= 0 || n > 500 {
		return def
	}
	return n
}

// parseDay reads a date (1 October 2026) or a full RFC 3339 time.
func parseDay(v string) (time.Time, error) {
	if t, err := time.Parse("2006-01-02", v); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, v)
}
