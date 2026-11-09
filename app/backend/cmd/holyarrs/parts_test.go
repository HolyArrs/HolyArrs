package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
)

// runApp starts the app and returns its address, its parts Control and a
// stop function that returns what serve returned.
func runApp(t *testing.T, c config) (string, *parts.Control, *jobs.Registry, func() error) {
	t.Helper()
	c.Addr = "127.0.0.1:0"
	type up struct {
		reg  *jobs.Registry
		addr string
	}
	ready := make(chan up, 1)
	var pc *parts.Control
	o := runOpts{ready: func(reg *jobs.Registry, addr string) { ready <- up{reg, addr} }, parts: func(c *parts.Control) { pc = c }}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, c, o) }()
	stopped := false
	stop := func() error {
		if stopped {
			return nil
		}
		stopped = true
		cancel()
		select {
		case err := <-done:
			return err
		case <-time.After(30 * time.Second):
			t.Fatal("the app did not stop")
		}
		return nil
	}
	t.Cleanup(func() { _ = stop() })
	select {
	case u := <-ready:
		return u.addr, pc, u.reg, stop
	case err := <-done:
		t.Fatalf("serve ended: %v", err)
	case <-time.After(60 * time.Second):
		t.Fatal("the app did not start")
	}
	return "", nil, nil, nil
}

// Every supervised loop, every stage on the health page and every Job kind
// with a Handler belongs to a part or is marked core in the parts table.
func TestEveryLoopAndJobKindHasAPart(t *testing.T) {
	var kinds []string
	var loops []string
	reg, _ := startApp(t, config{}, runOpts{kinds: func(k []string) { kinds = k },
		wrap: func(l jobs.Loop) jobs.Loop { loops = append(loops, l.Name); return l }})
	if len(kinds) < 20 || len(loops) < 5 {
		t.Fatalf("only %d kinds and %d loops", len(kinds), len(loops))
	}
	for _, k := range kinds {
		if _, ok := parts.JobKinds()[k]; !ok {
			t.Errorf("job kind %s belongs to no part in internal/core/parts/table.go", k)
		}
	}
	for _, l := range loops {
		if _, ok := parts.Loops()[l]; !ok {
			t.Errorf("loop %s belongs to no part in internal/core/parts/table.go", l)
		}
	}
	for _, st := range reg.List() {
		name, isStage := strings.CutPrefix(st.Key, "stage:")
		if !isStage {
			continue
		}
		if _, ok := parts.Loops()[name]; !ok {
			t.Errorf("stage %s belongs to no part in internal/core/parts/table.go", name)
		}
	}
}

// The part switches outlive a restart; safe mode forces every part off
// without changing what is saved.
func TestPartSwitchesSurviveARestart(t *testing.T) {
	dir := t.TempDir()
	_, pc, reg, stop := runApp(t, config{ConfigDir: dir})
	if err := pc.Set(context.Background(), parts.Music, false, domain.System); err != nil {
		t.Fatal(err)
	}
	if err := pc.Set(context.Background(), parts.Wishlist, false, domain.System); err != nil {
		t.Fatal(err)
	}
	waitState(t, reg, "wishlist", jobs.Off)
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	_, pc, reg, stop = runApp(t, config{ConfigDir: dir})
	if pc.On(parts.Music) || pc.On(parts.Wishlist) || !pc.On(parts.Movies) {
		t.Fatalf("after a restart, off: %v", pc.Off())
	}
	if st, _ := reg.Get(jobs.StageKey("wishlist")); st.State != jobs.Off || st.Reason != "Requests is off." {
		t.Fatalf("wishlist stage after a restart: %+v", st)
	}
	waitState(t, reg, "gates", jobs.Running)
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	_, pc, _, _ = runApp(t, config{ConfigDir: dir, SafeMode: true})
	if len(pc.Off()) != len(parts.All()) {
		t.Fatalf("safe mode: off %v", pc.Off())
	}
}

func waitState(t *testing.T, reg *jobs.Registry, stage string, want jobs.State) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		st, _ := reg.Get(jobs.StageKey(stage))
		if st.State == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: %+v, want %s", stage, st, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Restart in safe mode from the controls: the server stops cleanly with
// the restart exit status, and the next start (the restart policy's) is in
// safe mode once; the one after is normal.
func TestRestartInSafeModeFromTheControls(t *testing.T) {
	dir := t.TempDir()
	addr, _, _, stop := runApp(t, config{ConfigDir: dir})
	base := "http://" + addr
	jar, _ := cookiejar.New(nil)
	cl := &http.Client{Jar: jar}
	post := func(path, body, csrf string) (int, map[string]any) {
		req, _ := http.NewRequest("POST", base+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(accounts.CSRFHeader, csrf)
		resp, err := cl.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		var v map[string]any
		_ = json.Unmarshal(b, &v)
		return resp.StatusCode, v
	}
	st, v := post("/api/v1/auth/setup", `{"name":"luis","password":"a long test passphrase"}`, "")
	if st != 201 {
		t.Fatalf("setup %d %v", st, v)
	}
	csrf, _ := v["csrf"].(string)
	st, v = post("/api/v1/app/safe-mode", "", csrf)
	if st != 202 {
		t.Fatalf("ask %d %v", st, v)
	}
	st, v = post("/api/v1/app/safe-mode", `{"confirm":"`+v["confirm"].(string)+`"}`, csrf)
	if st != 202 || !strings.Contains(v["text"].(string), "restart policy") {
		t.Fatalf("confirm %d %v", st, v)
	}
	var ex errAppExit
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, safeNextFile)); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the next start was not marked as safe mode")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// serve ends on its own; stop only collects what it returned.
	time.Sleep(time.Second)
	if err := stop(); !errors.As(err, &ex) || ex.code != exitRestart {
		t.Fatalf("serve returned %v, want exit %d", err, exitRestart)
	}
	_, pc, reg, stop := runApp(t, config{ConfigDir: dir})
	if len(pc.Off()) != len(parts.All()) {
		t.Fatalf("the start after was not in safe mode: off %v", pc.Off())
	}
	if st, _ := reg.Get(jobs.StageKey("schedules")); st.State != jobs.Off {
		t.Fatalf("schedules in safe mode: %+v", st)
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	_, pc, _, _ = runApp(t, config{ConfigDir: dir})
	if len(pc.Off()) != 0 {
		t.Fatalf("the start after safe mode is not normal: off %v", pc.Off())
	}
}
