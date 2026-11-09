package health

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/logs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/updates"
)

// rigOpts is what a test changes in the health routes it serves.
type rigOpts struct {
	Engine    *Engine
	States    *jobs.Registry
	SafeMode  bool
	Queue     *jobs.Queue
	Parts     *parts.Control
	Secrets   *secrets.Vault
	Log       *logs.Log
	ConfigDir string
	SavePath  string
}

// rig serves every health route on one mux, wired the way the app wires
// them, with the other modules left out.
type rig struct {
	srv    *httptest.Server
	s      *database.Store
	q      *jobs.Queue
	mux    *http.ServeMux
	status *StatusCache
	sched  *ScheduleHandlers
}

func openStore(t *testing.T) *database.Store {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func execSQL(t *testing.T, s *database.Store, q string, args ...any) {
	t.Helper()
	if err := s.Write(context.Background(), func(tx *sql.Tx) error { _, err := tx.Exec(q, args...); return err }); err != nil {
		t.Fatal(err)
	}
}

func addMovieFolder(t *testing.T, s *database.Store) {
	t.Helper()
	execSQL(t, s, `INSERT INTO library_folders (created_at, created_by, updated_at, updated_by, path, kind, label, health, same_disk_as)
		VALUES ('2026-10-01T00:00:00Z', '{}', '2026-10-01T00:00:00Z', '{}', ?, 'movie', 'Movies', 'ok', '[]')`, t.TempDir())
}

func newRig(t *testing.T, s *database.Store, o rigOpts) *rig {
	t.Helper()
	q := o.Queue
	if q == nil {
		q = jobs.New(s, jobs.Options{Workers: 1, PollInterval: 5 * time.Millisecond})
	}
	eng := o.Engine
	if eng == nil {
		eng = &Engine{Store: s, Version: "test"}
	}
	uc := &updates.Checker{Store: s, Version: "test"}
	if eng.UpdateCheck == nil {
		eng.UpdateState = func(ctx context.Context) UpdateState {
			i := uc.Update(ctx)
			return UpdateState{On: i.On, Feed: i.Feed, CheckedAt: i.CheckedAt, Every: uc.Every(ctx), Latest: i.Latest, Newer: i.Newer}
		}
		eng.UpdateCheck = func(ctx context.Context) (string, bool, bool) {
			i, ran := uc.Scheduled(ctx)
			return i.Latest, i.Newer, ran
		}
	}
	r := &rig{s: s, q: q, mux: http.NewServeMux(), status: &StatusCache{}}
	h := &Handlers{Store: s, Engine: eng, States: o.States, SafeMode: o.SafeMode,
		DefaultFeed: updates.DefaultReleaseFeed, ResetUpdate: uc.ResetUpdate,
		SyncTrouble: func(context.Context) any { return []any{} }}
	st := &StatusHandlers{Store: s, Engine: eng, States: o.States,
		MetadataUp:  func(context.Context) (bool, bool) { return true, true },
		UpdateNewer: func(ctx context.Context) bool { return uc.Update(ctx).Newer }, Cache: r.status}
	if o.Parts != nil {
		h.PartStatuses = o.Parts.Statuses
		st.PartStatuses = o.Parts.Statuses
	}
	r.sched = &ScheduleHandlers{Store: s, Queue: q, States: o.States, Parts: o.Parts, SafeMode: o.SafeMode}
	app := &AppHandlers{Store: s, Version: "test", Started: time.Now(), Log: o.Log, Secrets: o.Secrets, ConfigDir: o.ConfigDir, SavePath: o.SavePath,
		HealthView: func(context.Context) any { return map[string]any{"status": "ok"} }}
	h.Routes(r.mux)
	st.Routes(r.mux)
	r.sched.Routes(r.mux)
	app.Routes(r.mux)
	(&UIHandlers{Store: s, Actor: func(*http.Request) domain.Actor { return domain.Actor{Kind: "user", ID: 1} }}).Routes(r.mux)
	(&updates.Handlers{Checker: uc}).Routes(r.mux)
	r.srv = httptest.NewServer(r.mux)
	t.Cleanup(r.srv.Close)
	return r
}

// setup serves the health routes on a store with one movie folder.
func setup(t *testing.T) (*httptest.Server, *database.Store, *jobs.Queue) {
	t.Helper()
	s := openStore(t)
	addMovieFolder(t, s)
	r := newRig(t, s, rigOpts{})
	return r.srv, r.s, r.q
}

// logServer serves the health routes with a vault, a log and a config folder.
func logServer(t *testing.T) (*httptest.Server, *secrets.Vault, *logs.Log) {
	t.Helper()
	dir := t.TempDir()
	s, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	key, _ := secrets.LoadKey(dir, "")
	v, err := secrets.Open(context.Background(), s, key)
	if err != nil {
		t.Fatal(err)
	}
	lg, err := logs.Open(filepath.Join(dir, "logs"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lg.Close() })
	addMovieFolder(t, s)
	r := newRig(t, s, rigOpts{Secrets: v, Log: lg, ConfigDir: dir, SavePath: "/downloads"})
	return r.srv, v, lg
}

// ctlFix serves the health routes with a parts Control: one Movies loop
// (gates) and one core loop (backups).
type ctlFix struct {
	srv  *httptest.Server
	s    *database.Store
	q    *jobs.Queue
	pc   *parts.Control
	sch  *ScheduleHandlers
	runs atomic.Int64
}

func newCtl(t *testing.T) *ctlFix { t.Helper(); return newCtlMode(t, false) }

func newCtlMode(t *testing.T, safe bool) *ctlFix {
	t.Helper()
	s := openStore(t)
	f := &ctlFix{s: s}
	reg := jobs.NewRegistry()
	f.q = jobs.New(s, jobs.Options{Workers: 1, PollInterval: 5 * time.Millisecond})
	f.pc = &parts.Control{Store: s, Reg: reg, Jobs: f.q, Drain: time.Second, SafeMode: safe, Logf: func(string, ...any) {}}
	if err := f.pc.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.q.SetGate(f.pc)
	for _, name := range []string{"gates", "backups"} {
		f.pc.Add(jobs.Loop{Name: name, Label: name, Every: 20 * time.Millisecond, Run: func(context.Context) error { f.runs.Add(1); return nil }}, name == "gates")
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel(); f.q.Stop(); reg.Wait() })
	f.pc.Start(ctx)
	if err := f.q.Start(ctx); err != nil {
		t.Fatal(err)
	}
	r := newRig(t, s, rigOpts{Queue: f.q, Parts: f.pc, States: reg, SafeMode: safe})
	f.srv, f.sch = r.srv, r.sched
	return f
}

func do(t *testing.T, method, url, body string, want int) []byte {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != want {
		t.Fatalf("%s %s: %d, want %d: %s", method, url, resp.StatusCode, want, b)
	}
	return b
}

func decodeJSON(t *testing.T, b []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("%s: %v", b, err)
	}
}
