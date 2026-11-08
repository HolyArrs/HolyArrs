package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"

	"github.com/HolyArrs/holyarrs/app/backend/internal/parts"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

type ctlFix struct {
	srv     *httptest.Server
	s       *database.Store
	pc      *parts.Control
	reg     *jobs.Registry
	api     *API
	runs    atomic.Int64 // passes of the test loops
	mu      sync.Mutex
	exits   []string
	exitErr error
}

func (f *ctlFix) exited() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.exits...)
}

// The control fixture stays here for the api tests that still use it
// (mcp, library summary); the control tests moved to cmd/holyarrs.

// newCtl serves the API with a parts Control (one Movies loop, gates, and
// one core loop, backups) and records whole-app exits instead of exiting.
func newCtl(t *testing.T, withAuth bool) *ctlFix {
	t.Helper()
	return newCtlMode(t, withAuth, false)
}

// newCtlMode is newCtl, in safe mode when safe is true.
func newCtlMode(t *testing.T, withAuth, safe bool) *ctlFix {
	t.Helper()
	dir := t.TempDir()
	s, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	f := &ctlFix{s: s, reg: jobs.NewRegistry()}
	q := jobs.New(s, jobs.Options{Workers: 1, PollInterval: 5 * time.Millisecond})
	f.pc = &parts.Control{Store: s, Reg: f.reg, Jobs: q, Drain: time.Second, SafeMode: safe, Logf: func(string, ...any) {}}
	if err := f.pc.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	q.SetGate(f.pc)
	for _, name := range []string{"gates", "backups"} {
		f.pc.Add(jobs.Loop{Name: name, Label: name, Every: 20 * time.Millisecond, Run: func(context.Context) error { f.runs.Add(1); return nil }}, name == "gates")
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel(); q.Stop(); f.reg.Wait() })
	f.pc.Start(ctx)
	if err := q.Start(ctx); err != nil {
		t.Fatal(err)
	}
	srvOpts := Server{Store: s, Queue: q, Metadata: metadata.Lab(), Version: "test", Parts: f.pc, States: f.reg, SafeMode: safe,
		ExitApp: func(how string) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.exitErr != nil {
				return f.exitErr
			}
			f.exits = append(f.exits, how)
			return nil
		}}
	if withAuth {
		srvOpts.Auth = accounts.New(s, accounts.Options{})
	}
	f.api = New(srvOpts)
	f.srv = httptest.NewServer(f.api.Handler())
	t.Cleanup(f.srv.Close)
	return f
}

func (f *ctlFix) events(t *testing.T) []domain.Event {
	t.Helper()
	es, _, err := f.s.Events(context.Background(), database.EventFilter{Kinds: []string{domain.EventControl}, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	return es
}

// confirmTwice runs a two-step control: the first call answers 202 with a
// confirm id and does nothing; the second, with the id, answers want.
func confirmTwice(t *testing.T, url string, want int) []byte {
	t.Helper()
	var ask struct {
		Confirm string `json:"confirm"`
		Expires int    `json:"expires_in_s"`
	}
	if err := json.Unmarshal(do(t, "POST", url, "", 202), &ask); err != nil || ask.Confirm == "" || ask.Expires != 60 {
		t.Fatalf("first step: %+v %v", ask, err)
	}
	return do(t, "POST", url, `{"confirm":"`+ask.Confirm+`"}`, want)
}
