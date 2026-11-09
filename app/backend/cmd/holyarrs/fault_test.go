package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
)

// The fault runs (3.7.10, decision 0007 "Containment"): a crash in every
// supervised loop, a slow write, bad outside services (each kind of linked
// app hung, then erroring) and a pass that ignores its deadline. In each,
// a fresh sign-in, the Library and a manual backup must still work, and
// the stage or link shows the state the run caused. With a file path in
// HOLYARRS_FAULT_OUT the times are written there for the release report.

// EnvFaultOut names a file the fault runs append their times to.
const EnvFaultOut = "HOLYARRS_FAULT_OUT"

// coreLimit is the longest a core action may take during a fault. It is
// generous on purpose (slow CI machines, the race detector); the measured
// times go to the report.
const coreLimit = 10 * time.Second

type faultApp struct {
	t    *testing.T
	base string
	dir  string
	pc   *parts.Control
	reg  *jobs.Registry
}

// startFaultApp runs the app with o and makes the owner.
func startFaultApp(t *testing.T, o runOpts) *faultApp {
	t.Helper()
	dir := t.TempDir()
	var pc *parts.Control
	o.parts = func(c *parts.Control) { pc = c }
	reg, addr := startApp(t, config{ConfigDir: dir}, o)
	a := &faultApp{t: t, base: "http://" + addr, dir: dir, pc: pc, reg: reg}
	c := newClient()
	if st, body, _ := a.call(c, "", "POST", "/api/v1/auth/setup", `{"name":"luis","password":"a long test passphrase"}`); st != 201 {
		t.Fatalf("setup %d %s", st, body)
	}
	return a
}

func newClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar, Timeout: 60 * time.Second}
}

func (a *faultApp) call(c *http.Client, csrf, method, path, body string) (int, string, time.Duration) {
	a.t.Helper()
	req, _ := http.NewRequest(method, a.base+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if csrf != "" {
		req.Header.Set(accounts.CSRFHeader, csrf)
	}
	began := time.Now()
	resp, err := c.Do(req)
	took := time.Since(began)
	if err != nil {
		a.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b), took
}

// coreWorks signs in afresh, reads the Library and makes a backup; each
// must answer within coreLimit. It returns the three times.
func (a *faultApp) coreWorks(run string) [3]time.Duration {
	a.t.Helper()
	c := newClient()
	st, body, signIn := a.call(c, "", "POST", "/api/v1/auth/login", `{"name":"luis","password":"a long test passphrase"}`)
	if st != 200 {
		a.t.Fatalf("%s: sign-in %d %s", run, st, body)
	}
	var v struct {
		CSRF string `json:"csrf"`
	}
	_ = json.Unmarshal([]byte(body), &v)
	st, body, library := a.call(c, v.CSRF, "GET", "/api/v1/titles", "")
	if st != 200 {
		a.t.Fatalf("%s: Library %d %s", run, st, body)
	}
	st, body, backup := a.call(c, v.CSRF, "POST", "/api/v1/backups", "")
	if st != 201 {
		a.t.Fatalf("%s: backup %d %s", run, st, body)
	}
	times := [3]time.Duration{signIn, library, backup}
	for i, what := range []string{"sign-in", "Library", "backup"} {
		if times[i] > coreLimit {
			a.t.Errorf("%s: %s took %v", run, what, times[i])
		}
	}
	report(a.t, fmt.Sprintf("| %s | %v | %v | %v |", run, signIn.Round(time.Millisecond), library.Round(time.Millisecond), backup.Round(time.Millisecond)))
	return times
}

var reportMu sync.Mutex

func report(t *testing.T, line string) {
	t.Helper()
	t.Log(line)
	out := os.Getenv(EnvFaultOut)
	if out == "" {
		return
	}
	reportMu.Lock()
	defer reportMu.Unlock()
	f, err := os.OpenFile(out, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	_, _ = fmt.Fprintln(f, line)
}

func waitStage(t *testing.T, reg *jobs.Registry, key string, ok func(jobs.Status) bool) jobs.Status {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		st, _ := reg.Get(key)
		if ok(st) {
			return st
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: %+v", key, st)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Every supervised loop crashes on every pass: while they all fail and
// retry, sign-in, the Library and a backup still work.
func TestFaultCrashInEveryLoop(t *testing.T) {
	reg := jobs.NewRegistry()
	reg.BaseDelay, reg.MaxDelay = 20*time.Millisecond, 100*time.Millisecond
	reg.Logf = func(string, ...any) {}
	wrap := func(l jobs.Loop) jobs.Loop {
		name := l.Name
		l.Run = func(context.Context) error { panic("injected crash in " + name) }
		return l
	}
	a := startFaultApp(t, runOpts{reg: reg, wrap: wrap})
	for _, name := range supervisedLoops {
		waitStage(t, reg, jobs.StageKey(name), func(s jobs.Status) bool { return s.Restarts >= 2 })
	}
	a.coreWorks("every loop crashing on every pass")
	for _, name := range supervisedLoops {
		st, _ := reg.Get(jobs.StageKey(name))
		if st.State != jobs.Failed && st.State != jobs.Retrying && st.State != jobs.Running {
			t.Errorf("%s: %+v", name, st)
		}
		if st.Restarts < 2 {
			t.Errorf("%s restarted %d times", name, st.Restarts)
		}
	}
}

// A write held open in the store for 5 s: the Library (reads) and a
// backup do not wait for it; sign-in waits only for that one write.
func TestFaultSlowWrite(t *testing.T) {
	a := startFaultApp(t, runOpts{})
	held := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- a.pc.Store.Write(context.Background(), func(*sql.Tx) error {
			close(held)
			time.Sleep(5 * time.Second)
			return nil
		})
	}()
	<-held
	// SQLite has one writer: the sign-in's session write cannot overtake a
	// write already running, so it waits for it (at most the 20 s write
	// cap), then goes first, ahead of any other waiting write.
	times := a.coreWorks("a write held for 5 s")
	if times[0] > 8*time.Second {
		t.Errorf("sign-in took %v: more than the held write and a little", times[0])
	}
	if err := <-done; err != nil {
		t.Fatalf("held write: %v", err)
	}
}

// badService answers nothing until released, then a 500 to everything.
type badService struct {
	*httptest.Server
	release chan struct{}
	once    sync.Once
	calls   atomic.Int32
}

func newBadService(t *testing.T) *badService {
	b := &badService{release: make(chan struct{})}
	b.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.calls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-b.release:
		case <-r.Context().Done():
			return
		}
		http.Error(w, "broken on purpose", http.StatusInternalServerError)
	}))
	t.Cleanup(b.Close)
	t.Cleanup(b.answer)
	return b
}

// answer lets every held call, and every later one, answer the 500.
func (b *badService) answer() { b.once.Do(func() { close(b.release) }) }

// Each kind of linked app (a Source, a Downloader, a subtitle site, a
// media server) hangs and then errors: while their tests hang, sign-in,
// the Library and a backup work, and afterwards the Downloader and the
// media server show a state other than running.
func TestFaultBadOutsideServices(t *testing.T) {
	a := startFaultApp(t, runOpts{})
	c := newClient()
	st, body, _ := a.call(c, "", "POST", "/api/v1/auth/login", `{"name":"luis","password":"a long test passphrase"}`)
	if st != 200 {
		t.Fatalf("sign-in %d %s", st, body)
	}
	var who struct {
		CSRF string `json:"csrf"`
	}
	_ = json.Unmarshal([]byte(body), &who)
	type link struct {
		kind, add, body, test string
		svc                   *badService
		id                    int64
	}
	links := []*link{
		{kind: "source", add: "/api/v1/sources", body: `{"name":"Hung Source","format":"torznab","url":"%s/api","api_key":"k-test","allow_local_network":true}`, test: "/api/v1/sources/%d/test"},
		{kind: "downloader", add: "/api/v1/downloaders", body: `{"name":"Hung Downloader","kind":"qbittorrent","url":"%s","secret":"your-test-key","allow_local_network":true}`, test: "/api/v1/downloaders/%d/test"},
		{kind: "subtitle_site", add: "/api/v1/subtitle-sites", body: `{"kind":"fake","url":"%s","allow_local_network":true,"limits":{"gap_ms":1,"per_window":100,"window_seconds":3600}}`, test: "/api/v1/subtitle-sites/%d/test"},
		{kind: "media_server", add: "/api/v1/media-servers", body: `{"name":"Hung Jellyfin","kind":"jellyfin","url":"%s","token":"tk-test","allow_local_network":true}`, test: "/api/v1/media-servers/%d/test"},
	}
	for _, l := range links {
		l.svc = newBadService(t)
		st, body, _ := a.call(c, who.CSRF, "POST", l.add, fmt.Sprintf(l.body, l.svc.URL))
		if st != 201 {
			t.Fatalf("add %s: %d %s", l.kind, st, body)
		}
		var v struct {
			ID int64 `json:"id"`
		}
		_ = json.Unmarshal([]byte(body), &v)
		if v.ID == 0 {
			t.Fatalf("add %s: %s", l.kind, body)
		}
		l.id = v.ID
	}
	// Test every link now: each call hangs on its bad service.
	var wg sync.WaitGroup
	for _, l := range links {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest("POST", a.base+fmt.Sprintf(l.test, l.id), nil)
			req.Header.Set(accounts.CSRFHeader, who.CSRF)
			if resp, err := c.Do(req); err == nil {
				resp.Body.Close()
			}
		}()
	}
	deadline := time.Now().Add(20 * time.Second)
	for _, l := range links {
		for l.svc.calls.Load() == 0 {
			if time.Now().After(deadline) {
				t.Fatalf("the %s test never reached its service", l.kind)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	a.coreWorks("a Source, a Downloader, a subtitle site and a media server hung")
	for _, l := range links {
		l.svc.answer()
	}
	wg.Wait()
	a.coreWorks("the same four answering 500")
	st, body, _ = a.call(c, who.CSRF, "GET", "/api/v1/health/states", "")
	if st != 200 {
		t.Fatalf("health states %d %s", st, body)
	}
	var states struct {
		Links []jobs.Status `json:"states"`
	}
	if err := json.Unmarshal([]byte(body), &states); err != nil {
		t.Fatalf("health states: %v %s", err, body)
	}
	for _, l := range links {
		key := fmt.Sprintf("link:%s:%d", l.kind, l.id)
		found := false
		for _, s := range states.Links {
			if s.Key != key {
				continue
			}
			found = true
			report(t, fmt.Sprintf("| %s after hanging, then 500 | %s | %s |", l.kind, s.State, s.Reason))
			// A failed "test now" does not move a Source or a subtitle
			// site off running: their state follows failed searches
			// (back-off), which this run does not make.
			if s.State == jobs.Running && (l.kind == "downloader" || l.kind == "media_server") {
				t.Errorf("%s still shows running after hanging and erroring", key)
			}
		}
		if !found {
			t.Errorf("%s is not on the health states: %s", key, body)
		}
	}
}

// A pass that ignores its deadline is shown stuck and never started a
// second time; sign-in, the Library and a backup still work.
func TestFaultPassIgnoresItsDeadline(t *testing.T) {
	reg := jobs.NewRegistry()
	reg.Logf = func(string, ...any) {}
	block := make(chan struct{})
	var starts atomic.Int32
	wrap := func(l jobs.Loop) jobs.Loop {
		if l.Name != "gates" {
			return l
		}
		l.Every, l.Deadline = 50*time.Millisecond, 50*time.Millisecond
		l.Run = func(context.Context) error {
			starts.Add(1)
			<-block // ignores its context
			return nil
		}
		return l
	}
	a := startFaultApp(t, runOpts{reg: reg, wrap: wrap})
	t.Cleanup(func() { close(block) }) // runs before the app stops
	st := waitStage(t, reg, jobs.StageKey("gates"), func(s jobs.Status) bool { return s.Stuck })
	if st.State != jobs.Failed || !strings.Contains(st.Reason, "Stuck") {
		t.Errorf("stuck stage: %+v", st)
	}
	a.coreWorks("one pass ignoring its deadline")
	time.Sleep(500 * time.Millisecond) // ten more periods
	if n := starts.Load(); n != 1 {
		t.Errorf("the stuck pass was started %d times", n)
	}
}
