package health

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/logs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/updates"
)

// route: GET /api/v1/app/storage
func TestStoragePaths(t *testing.T) {
	t.Parallel()
	srv, _, _ := logServer(t)
	var got struct {
		App     []PathInfo `json:"app"`
		Folders []PathInfo `json:"library_folders"`
		Save    string     `json:"save_path"`
	}
	if err := json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/app/storage", "", 200), &got); err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, p := range got.App {
		names[p.Name] = true
		if p.Path == "" {
			t.Fatalf("no path: %+v", p)
		}
	}
	for _, n := range []string{"Config folder", "Main database", "History database", "Backups", "Artwork", "Log files"} {
		if !names[n] {
			t.Errorf("missing %s in %+v", n, got.App)
		}
	}
	if len(got.Folders) != 1 || got.Folders[0].Kind != "movie" || got.Folders[0].Name != "Movies" || got.Save != "/downloads" {
		t.Fatalf("folders %+v save %q", got.Folders, got.Save)
	}
	if got.App[1].Bytes == 0 {
		t.Fatalf("database size unknown: %+v", got.App[1])
	}
}

// route: GET /api/v1/logs
// route: GET /api/v1/logs/download
func TestLogViewerNeverShowsAVaultSecret(t *testing.T) {
	t.Parallel()
	srv, v, lg := logServer(t)
	const secret = "VAULT_ONLY_TEST_VALUE"
	// Saved after the log last learned the vault's values: the line is
	// written unmasked by the value, and must still never be shown.
	if err := v.SetSetting(context.Background(), "links", "tmdb_token", secret, domain.System); err != nil {
		t.Fatal(err)
	}
	l := log.New(lg, "", log.LstdFlags)
	l.Printf("tmdb: answered 401 for %s", secret)
	l.Printf("downloader: qBittorrent failed: connection refused")
	l.Printf("http://admin:<pw>@10.0.0.5:8080 asked with apikey=XXXX_KEY today")
	var got struct {
		Lines []logs.Line `json:"lines"`
		Total int         `json:"total"`
	}
	b := do(t, "GET", srv.URL+"/api/v1/logs", "", 200)
	if strings.Contains(string(b), secret) || strings.Contains(string(b), "admin:<pw>") || strings.Contains(string(b), "XXXX_KEY") {
		t.Fatalf("secret shown: %s", b)
	}
	if err := json.Unmarshal(b, &got); err != nil || got.Total != 3 || got.Lines[0].At.IsZero() {
		t.Fatalf("lines: %v %s", err, b)
	}
	if err := json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/logs?level=error", "", 200), &got); err != nil || got.Total != 1 || got.Lines[0].Level != "error" {
		t.Fatalf("level: %+v", got)
	}
	if err := json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/logs?q=TMDB&from=2000-01-01&to=2999-01-01&offset=0&limit=10", "", 200), &got); err != nil || got.Total != 1 {
		t.Fatalf("text: %+v", got)
	}
	for _, bad := range []string{"level=loud", "from=yesterday", "to=x", "offset=-1", "q=" + strings.Repeat("x", 201)} {
		do(t, "GET", srv.URL+"/api/v1/logs?"+bad, "", 400)
	}
	resp, err := http.Get(srv.URL + "/api/v1/logs/download")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body := make([]byte, 1<<16)
	n, _ := resp.Body.Read(body)
	text := string(body[:n])
	if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Content-Disposition"), "attachment") || strings.Contains(text, secret) ||
		!strings.Contains(text, "connection refused") || strings.Contains(text, "admin:<pw>") {
		t.Fatalf("download %d %q: %s", resp.StatusCode, resp.Header.Get("Content-Disposition"), text)
	}
}

func TestLogsWithoutALogFile(t *testing.T) {
	t.Parallel()
	srv, _, _ := setup(t)
	do(t, "GET", srv.URL+"/api/v1/logs", "", 503)
	do(t, "GET", srv.URL+"/api/v1/logs/download", "", 503)
}

// route: GET /api/v1/app/update
// route: POST /api/v1/app/update/check
func TestUpdateNoticeOverHTTP(t *testing.T) {
	t.Parallel()
	var calls int
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"tag_name":"v9.0.0","html_url":"https://example.com/v9"}`))
	}))
	defer feed.Close()
	srv, _, _ := setup(t)
	if b := do(t, "GET", srv.URL+"/api/v1/app/update", "", 200); !strings.Contains(string(b), `"on":true`) || calls != 0 {
		t.Fatalf("get: %s", b)
	}
	do(t, "PUT", srv.URL+"/api/v1/settings/health", `{"every_minutes":5,"flap_guard":2,"low_space_mb":1024,"update_check_url":"`+feed.URL+`"}`, 200)
	if b := do(t, "POST", srv.URL+"/api/v1/app/update/check", "", 200); !strings.Contains(string(b), `"latest":"v9.0.0"`) {
		t.Fatalf("check: %s", b)
	}
	do(t, "PUT", srv.URL+"/api/v1/settings/health", `{"every_minutes":5,"flap_guard":2,"low_space_mb":1024,"update_check_url":"`+feed.URL+`","update_check":false}`, 200)
	if b := do(t, "POST", srv.URL+"/api/v1/app/update/check", "", 200); !strings.Contains(string(b), `"on":false`) || calls != 1 {
		t.Fatalf("off: %d %s", calls, b)
	}
}

func TestUpdateFeedHoldsNoSecret(t *testing.T) {
	t.Parallel()
	srv, _, _ := setup(t)
	for _, bad := range []string{"https://x.example/feed?token=XXXX_T", "https://user:XXXX@x.example/feed", "https://x.example/feed#frag"} {
		do(t, "PUT", srv.URL+"/api/v1/settings/health", `{"every_minutes":5,"flap_guard":2,"low_space_mb":1024,"update_check_url":"`+bad+`"}`, 400)
	}
	r := httptest.NewRequest("GET", "/", nil)
	in := updates.UpdateInfo{Feed: "https://user:XXXX@x.example/feed?" + "to" + "ken=XXXX_T#f"}
	if got := updates.ViewFor(r, in).Feed; got != "https://x.example/feed" {
		t.Fatalf("owner sees %q", got)
	}
	mgr := r.WithContext(webserver.ContextWith(r.Context(), webserver.Caller{UserID: 2, Role: webserver.RoleMember, Perms: []webserver.Perm{webserver.PermManageLibrary}}))
	if got := updates.ViewFor(mgr, in).Feed; got != "" {
		t.Fatalf("manager sees %q", got)
	}
}

// route: GET /api/v1/app/about
func TestAboutOverHTTP(t *testing.T) {
	t.Parallel()
	srv, _, _ := setup(t)
	b := string(do(t, "GET", srv.URL+"/api/v1/app/about", "", 200))
	for _, want := range []string{`"go":"go`, `"sqlite":"3.`, `"migration":`, `"uptime_s":`} {
		if !strings.Contains(b, want) {
			t.Fatalf("missing %s in %s", want, b)
		}
	}
}
