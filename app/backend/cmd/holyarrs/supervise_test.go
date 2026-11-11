package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
)

// startApp runs the whole app in a temp config folder and returns its
// Registry and address; the app stops when the test ends.
func startApp(t *testing.T, c config, o runOpts) (*jobs.Registry, string) {
	t.Helper()
	if c.ConfigDir == "" {
		c.ConfigDir = t.TempDir()
	}
	c.Addr = "127.0.0.1:0"
	type up struct {
		reg  *jobs.Registry
		addr string
	}
	ready := make(chan up, 1)
	o.ready = func(reg *jobs.Registry, addr string) { ready <- up{reg, addr} }
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, c, o) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("serve: %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Error("the app did not stop")
		}
	})
	select {
	case u := <-ready:
		return u.reg, u.addr
	case err := <-done:
		t.Fatalf("serve ended: %v", err)
	case <-time.After(60 * time.Second):
		t.Fatal("the app did not start")
	}
	return nil, ""
}

// supervisedLoops are every background loop main starts under the supervisor.
var supervisedLoops = []string{"backups", "downloads", "schedules", "stranded_imports", "gates", "sessions", "wishlist", "log_secrets", "music_warm", "trailer_pass"}

// A crash injected into each loop leaves the program running and the stage
// failed, then retrying, then running again.
func TestCrashInEveryLoopIsContained(t *testing.T) {
	reg := jobs.NewRegistry()
	reg.BaseDelay, reg.MaxDelay = 50*time.Millisecond, 200*time.Millisecond
	reg.Logf = func(string, ...any) {}
	var mu sync.Mutex
	seen := map[string][]jobs.State{}
	reg.OnChange = func(s jobs.Status) {
		mu.Lock()
		seen[s.Key] = append(seen[s.Key], s.State)
		mu.Unlock()
	}
	started := map[string]bool{}
	var startedMu sync.Mutex
	wrap := func(l jobs.Loop) jobs.Loop {
		startedMu.Lock()
		started[l.Name] = true
		startedMu.Unlock()
		var n atomic.Int32
		run := l.Run
		l.Run = func(ctx context.Context) error {
			if n.Add(1) == 1 {
				panic("injected crash in " + l.Name)
			}
			return run(ctx)
		}
		return l
	}
	reg, addr := startApp(t, config{}, runOpts{reg: reg, wrap: wrap})
	for _, name := range supervisedLoops {
		key := jobs.StageKey(name)
		deadline := time.Now().Add(20 * time.Second)
		for {
			st, _ := reg.Get(key)
			if st.Restarts >= 1 && st.State == jobs.Running {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s did not come back: %+v", name, st)
			}
			time.Sleep(10 * time.Millisecond)
		}
		mu.Lock()
		got := seen[key]
		mu.Unlock()
		if !hasInOrder(got, jobs.Failed, jobs.Retrying, jobs.Running) {
			t.Fatalf("%s states %v: want failed, then retrying, then running", name, got)
		}
	}
	startedMu.Lock()
	for _, name := range supervisedLoops {
		if !started[name] {
			t.Errorf("loop %s was not started", name)
		}
	}
	if len(started) != len(supervisedLoops) {
		t.Errorf("started %v; add new loops to supervisedLoops", started)
	}
	startedMu.Unlock()
	resp, err := http.Get("http://" + addr + "/healthz")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("app not answering after the crashes: %v", err)
	}
	resp.Body.Close()
}

func hasInOrder(got []jobs.State, want ...jobs.State) bool {
	i := 0
	for _, s := range got {
		if i < len(want) && s == want[i] {
			i++
		}
	}
	return i == len(want)
}

// Every Job kind with a Handler has its own row in the pool table.
func TestEveryJobKindHasAPool(t *testing.T) {
	var kinds []string
	startApp(t, config{}, runOpts{kinds: func(k []string) { kinds = k }})
	if len(kinds) < 20 {
		t.Fatalf("only %d kinds: %v", len(kinds), kinds)
	}
	for _, k := range kinds {
		if !jobs.Assigned(k) {
			t.Errorf("job kind %s has no pool in internal/jobs/pools.go", k)
		}
	}
}

// Safe mode starts only the core: sign-in works, the core stages run, every
// optional stage is off with a reason and schedules are not started; the
// sign-in page and the health states say so.
func TestSafeModeStartsOnlyTheCore(t *testing.T) {
	reg, addr := startApp(t, config{SafeMode: true}, runOpts{})
	base := "http://" + addr
	for _, name := range []string{"schedules", "downloads", "stranded_imports", "gates", "wishlist", "jobs_io", "jobs_heavy", "jobs_migrate", "media_servers"} {
		st, _ := reg.Get(jobs.StageKey(name))
		if st.State != jobs.Off || !strings.Contains(st.Reason, "Safe mode") {
			t.Errorf("%s: %+v", name, st)
		}
	}
	for _, name := range []string{"backups", "sessions", "jobs_core"} {
		deadline := time.Now().Add(10 * time.Second)
		for {
			st, _ := reg.Get(jobs.StageKey(name))
			if st.State == jobs.Running {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s: %+v", name, st)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	resp, err := http.Get(base + "/login")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("sign-in page in safe mode: %v %v", err, resp)
	}
	resp.Body.Close()
	resp, err = http.Get(base + "/api/v1/auth/state")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), `"safe_mode":true`) {
		t.Fatalf("auth state: %s", b)
	}
}

// Safe mode comes from the flag or the environment variable.
func TestSafeModeFlagAndEnv(t *testing.T) {
	c, err := parseFlags([]string{"-safe-mode"}, io.Discard)
	if err != nil || !c.SafeMode {
		t.Fatalf("flag: %+v %v", c, err)
	}
	t.Setenv(EnvSafeMode, "1")
	if c, _ := parseFlags(nil, io.Discard); !c.SafeMode {
		t.Fatal("env 1 did not turn on safe mode")
	}
	t.Setenv(EnvSafeMode, "0")
	if c, _ := parseFlags(nil, io.Discard); c.SafeMode {
		t.Fatal("env 0 turned on safe mode")
	}
}
