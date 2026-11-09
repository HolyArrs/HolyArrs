package health

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

func gapStore(t *testing.T) *database.Store {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func gapServe(h *Handlers, method, path, body string) *httptest.ResponseRecorder {
	m := http.NewServeMux()
	h.Routes(m)
	rr := httptest.NewRecorder()
	m.ServeHTTP(rr, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rr
}

func gapEngine(s *database.Store) *Engine {
	return &Engine{Store: s, Version: "test", Outside: func() []outbound.State { return nil }}
}

func TestPutSettingsResetsUpdate(t *testing.T) {
	t.Parallel()
	s := gapStore(t)
	called := 0
	h := &Handlers{Store: s, Engine: gapEngine(s), SyncTrouble: func(context.Context) any { return nil }, ResetUpdate: func(context.Context) { called++ }}
	if rr := gapServe(h, "PUT", "/api/v1/settings/health", `{"every_minutes":5,"flap_guard":2,"low_space_mb":1024}`); rr.Code != 200 || called != 1 {
		t.Fatalf("good put: %d %s called=%d", rr.Code, rr.Body, called)
	}
	if rr := gapServe(h, "PUT", "/api/v1/settings/health", `{"every_minutes":0,"flap_guard":2}`); rr.Code != 400 || called != 1 {
		t.Fatalf("bad put: %d called=%d", rr.Code, called)
	}
}

func TestChecksIncludesSyncTrouble(t *testing.T) {
	t.Parallel()
	s := gapStore(t)
	h := &Handlers{Store: s, Engine: gapEngine(s), SyncTrouble: func(context.Context) any { return []string{"qbit"} }}
	rr := gapServe(h, "GET", "/api/v1/health/checks", "")
	var out map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil || rr.Code != 200 {
		t.Fatalf("%d %s %v", rr.Code, rr.Body, err)
	}
	if sy, _ := out["sync"].([]any); len(sy) != 1 || sy[0] != "qbit" {
		t.Fatalf("sync %v", out["sync"])
	}
	if _, ok := out["worst"]; !ok {
		t.Fatalf("no worst: %v", out)
	}
	// SyncTrouble is required wiring; without it the handler panics today.
	t.Run("nil", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Log("nil SyncTrouble no longer panics; turn this into a 200 check")
			}
		}()
		gapServe(&Handlers{Store: s, Engine: gapEngine(s)}, "GET", "/api/v1/health/checks", "")
	})
}

func TestStatesMergesPartsAndLinks(t *testing.T) {
	t.Parallel()
	s := gapStore(t)
	reg := jobs.NewRegistry()
	reg.Set(jobs.StageKey("s1"), "stage", "S1", jobs.Retrying, "x")
	execSQL(t, s, `INSERT INTO downloaders (created_at, created_by, updated_at, updated_by, name, adapter, protocol, url, category, priority, path_map, after_import, state)
		VALUES ('2026-10-01T00:00:00Z', '{}', '2026-10-01T00:00:00Z', '{}', 'Lab qBittorrent', 'qbittorrent', 'torrent', 'http://qb', 'holyarrs', 1, '[]', 'leave', 'auth_failed')`)
	h := &Handlers{Store: s, Engine: gapEngine(s), States: reg, SafeMode: true,
		PartStatuses: func() []jobs.Status { return []jobs.Status{{Key: "part:music", Name: "part:music"}} }}
	rr := gapServe(h, "GET", "/api/v1/health/states", "")
	var got struct {
		SafeMode bool          `json:"safe_mode"`
		States   []jobs.Status `json:"states"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, st := range got.States {
		keys = append(keys, st.Key)
	}
	if strings.Join(keys, ",") != "stage:s1,part:music,link:downloader:1" || got.SafeMode != h.SafeMode {
		t.Fatalf("%v safe=%v", keys, got.SafeMode)
	}
}

func TestLibraryFoldersOrderAndEmpty(t *testing.T) {
	t.Parallel()
	s := gapStore(t)
	ctx := context.Background()
	if ds, err := libraryFolders(ctx, s.Read()); err != nil || len(ds) != 0 {
		t.Fatalf("empty: %v %v", ds, err)
	}
	h := &Handlers{Store: s, Engine: gapEngine(s)}
	if rr := gapServe(h, "GET", "/api/v1/health/disks", ""); rr.Body.String() != `{"disks":[]}`+"\n" {
		t.Fatalf("empty body %q", rr.Body)
	}
	err := s.Write(ctx, func(tx *sql.Tx) error {
		for _, f := range [][3]string{{"/b", "show", "B"}, {"/a", "show", "A"}, {"/m", "movie", "M"}} {
			if _, err := tx.Exec(`INSERT INTO library_folders (created_at, created_by, updated_at, updated_by, path, kind, label, health, same_disk_as)
				VALUES ('t', '{}', 't', '{}', ?, ?, ?, 'ok', '[]')`, f[0], f[1], f[2]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ds, err := libraryFolders(ctx, s.Read())
	if err != nil || len(ds) != 3 || ds[0].Path != "/m" || ds[1].Path != "/a" || ds[2].Path != "/b" || ds[1].Label != "A" {
		t.Fatalf("%+v %v", ds, err)
	}
}

func TestNotOwnerAndKeyKitWords(t *testing.T) {
	t.Parallel()
	req := func(role string) *http.Request {
		r := httptest.NewRequest("GET", "/", nil)
		if role == "" {
			return r
		}
		return r.WithContext(webserver.ContextWith(r.Context(), webserver.Caller{Role: role}))
	}
	for role, want := range map[string]bool{"": false, webserver.RoleOwner: false, webserver.RoleAdmin: true} {
		if got := NotOwner(req(role)); got != want {
			t.Errorf("NotOwner(%q)=%v", role, got)
		}
	}
	kit := func(state string) []domain.HealthCheck {
		return []domain.HealthCheck{{Key: "backups:key", State: state, Message: "x"}}
	}
	cs := kit(Warning)
	ownerWords(req(webserver.RoleAdmin), cs)
	if cs[0].Message != KeyKitForOthers {
		t.Fatalf("admin: %q", cs[0].Message)
	}
	cs = kit(Warning)
	ownerWords(req(webserver.RoleOwner), cs)
	if cs[0].Message != "x" {
		t.Fatalf("owner: %q", cs[0].Message)
	}
	cs = kit(OK)
	ownerWords(req(webserver.RoleAdmin), cs)
	if cs[0].Message != "x" {
		t.Fatalf("ok state: %q", cs[0].Message)
	}
}
