package health

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

// route: GET /api/v1/health/checks
// route: POST /api/v1/health/checks/run
func TestHealthChecksOverHTTP(t *testing.T) {
	t.Parallel()
	srv, s, _ := setup(t)
	if b := do(t, "GET", srv.URL+"/api/v1/health/checks", "", 200); !strings.Contains(string(b), `"checks":[]`) || !strings.Contains(string(b), `"worst":"ok"`) {
		t.Fatalf("before a run: %s", b)
	}
	execSQL(t, s, `INSERT INTO downloaders (created_at, created_by, updated_at, updated_by, name, adapter, protocol, url, category, priority, path_map, after_import, state)
		VALUES ('2026-10-01T00:00:00Z', '{}', '2026-10-01T00:00:00Z', '{}', 'Lab qBittorrent', 'qbittorrent', 'torrent', 'http://qb', 'holyarrs', 1, '[]', 'leave', 'unreachable')`)
	var got struct {
		Checks []struct {
			Key, Kind, Subject, State, Message string
		} `json:"checks"`
		Worst string `json:"worst"`
	}
	b := do(t, "POST", srv.URL+"/api/v1/health/checks/run", "", 200)
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	// Other checks, including free disk space, depend on the test host. Find
	// this fixture's Downloader by identity rather than assuming it sorts first.
	downloaderWarning := false
	for _, check := range got.Checks {
		if check.Key == "downloader:1" && check.State == "warning" && check.Message != "" {
			downloaderWarning = true
		}
	}
	if got.Worst != "warning" || !downloaderWarning {
		t.Fatalf("after a run: %s", b)
	}
	if b := do(t, "GET", srv.URL+"/api/v1/health/checks", "", 200); !strings.Contains(string(b), "Lab qBittorrent does not answer") {
		t.Fatalf("stored: %s", b)
	}
}

// route: GET /api/v1/settings/health
// route: PUT /api/v1/settings/health
func TestHealthSettingsOverHTTP(t *testing.T) {
	t.Parallel()
	srv, _, _ := setup(t)
	if b := do(t, "GET", srv.URL+"/api/v1/settings/health", "", 200); string(b) != `{"every_minutes":5,"flap_guard":2,"low_space_mb":1024,"update_check_url":"","update_check":true,"update_check_local":false,"update_prerelease":false,"update_check_every_days":1,"update_check_default":"https://api.github.com/repos/HolyArrs/HolyArrs/releases/latest"}`+"\n" {
		t.Fatalf("defaults %s", b)
	}
	b := do(t, "PUT", srv.URL+"/api/v1/settings/health", `{"every_minutes":15,"flap_guard":3,"low_space_mb":2048,"update_check_url":"https://example.com/latest"}`, 200)
	if !strings.Contains(string(b), `"every_minutes":15`) || !strings.Contains(string(b), `"flap_guard":3`) || !strings.Contains(string(b), "example.com/latest") {
		t.Fatalf("saved %s", b)
	}
	b = do(t, "PUT", srv.URL+"/api/v1/settings/health", `{"every_minutes":15,"flap_guard":3,"low_space_mb":2048,"update_check_url":"","update_check_every_days":7}`, 200)
	if !strings.Contains(string(b), `"update_check_every_days":7`) {
		t.Fatalf("days %s", b)
	}
	for _, bad := range []string{`{"every_minutes":0,"flap_guard":2}`, `{"every_minutes":5,"flap_guard":0}`, `{"every_minutes":5,"flap_guard":2,"update_check_url":"ftp://x"}`, `{"every_minutes":5,"flap_guard":2,"update_check_every_days":2}`, `nope`} {
		do(t, "PUT", srv.URL+"/api/v1/settings/health", bad, 400)
	}
}

// route: GET /api/v1/health/states
func TestHealthStatesOverHTTP(t *testing.T) {
	t.Parallel()
	reg := jobs.NewRegistry()
	reg.Set(jobs.StageKey("backups"), "stage", "Backups", jobs.Retrying, "the disk is full")
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	// Other tests in this package trip breakers in the process-wide outside
	// registry; read an empty one so only this fixture's states show.
	eng := &Engine{Store: s, Version: "test", Outside: func() []outbound.State { return nil }}
	srv := newRig(t, s, rigOpts{Engine: eng, States: reg, SafeMode: true}).srv
	execSQL(t, s, `INSERT INTO downloaders (created_at, created_by, updated_at, updated_by, name, adapter, protocol, url, category, priority, path_map, after_import, state)
		VALUES ('2026-10-01T00:00:00Z', '{}', '2026-10-01T00:00:00Z', '{}', 'Lab qBittorrent', 'qbittorrent', 'torrent', 'http://qb', 'holyarrs', 1, '[]', 'leave', 'auth_failed')`)
	var got struct {
		SafeMode bool          `json:"safe_mode"`
		States   []jobs.Status `json:"states"`
	}
	if err := json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/health/states", "", 200), &got); err != nil {
		t.Fatal(err)
	}
	if !got.SafeMode || len(got.States) != 2 {
		t.Fatalf("%+v", got)
	}
	if st := got.States[0]; st.Key != "stage:backups" || st.State != jobs.Retrying || st.Reason != "the disk is full" {
		t.Fatalf("stage %+v", st)
	}
	if l := got.States[1]; l.Key != "link:downloader:1" || l.State != jobs.Failed || !strings.Contains(l.Reason, "turned down the sign-in") {
		t.Fatalf("link %+v", l)
	}
	// Without a Registry the list still answers, as an empty list.
	s2, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s2.Close() })
	srv2 := newRig(t, s2, rigOpts{Engine: &Engine{Store: s2, Version: "test", Outside: func() []outbound.State { return nil }}}).srv
	if b := do(t, "GET", srv2.URL+"/api/v1/health/states", "", 200); string(b) != `{"safe_mode":false,"states":[]}`+"\n" {
		t.Fatalf("empty: %s", b)
	}
}

// route: GET /api/v1/health/disks
func TestHealthDisksOverHTTP(t *testing.T) {
	t.Parallel()
	srv, _, _ := setup(t)
	if b := do(t, "GET", srv.URL+"/api/v1/health/disks", "", 200); !strings.Contains(string(b), `"total_bytes":`) || !strings.Contains(string(b), `"kind":"movie"`) {
		t.Fatalf("disks: %s", b)
	}
}
