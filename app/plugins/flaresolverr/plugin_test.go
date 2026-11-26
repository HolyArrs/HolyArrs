package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Secrets a job can carry. None of them may reach the log.
const (
	pageURL     = "https://site.example/api?t=search&q=dune&apikey=KEY123SECRET"
	proxyPass   = "PROXYPASSSECRET"
	cookieValue = "COOKIESECRET"
	agent       = "Mozilla/5.0 Test"
)

// fakeHook is HolyArrs's side of the botcheck hook: it hands out the jobs
// queued in it, then answers 204, and keeps every answer posted back.
type fakeHook struct {
	t       *testing.T
	mu      sync.Mutex
	jobs    []job
	answers []map[string]json.RawMessage
	polls   atomic.Int32
	got     chan struct{}
	srv     *httptest.Server
}

func newFakeHook(t *testing.T, jobs ...job) *fakeHook {
	h := &fakeHook{t: t, jobs: jobs, got: make(chan struct{}, 16)}
	h.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/plugin-hooks/botcheck" || r.Header.Get("Authorization") != "Bearer hap_test" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		switch r.Method {
		case http.MethodGet:
			h.polls.Add(1)
			h.mu.Lock()
			defer h.mu.Unlock()
			if len(h.jobs) == 0 {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			j := h.jobs[0]
			h.jobs = h.jobs[1:]
			_ = json.NewEncoder(w).Encode(j)
		case http.MethodPost:
			var a map[string]json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
				t.Errorf("answer: %v", err)
			}
			h.mu.Lock()
			h.answers = append(h.answers, a)
			h.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			h.got <- struct{}{}
		}
	}))
	t.Cleanup(h.srv.Close)
	return h
}

// wait waits for n answers and returns them.
func (h *fakeHook) wait(n int) []map[string]json.RawMessage {
	h.t.Helper()
	for range n {
		select {
		case <-h.got:
		case <-time.After(5 * time.Second):
			h.t.Fatalf("no answer came")
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]map[string]json.RawMessage(nil), h.answers...)
}

func openJob(id string) job {
	p, _ := json.Marshal(map[string]any{"url": pageURL, "proxy": map[string]string{"url": "socks5://vpn:1080", "username": "me", "password": proxyPass}})
	return job{ID: id, Op: "open", Payload: p}
}

// startPlugin runs the plugin against hook and the FlareSolverr at address
// until the test ends, with short waits, and returns what it logged.
func startPlugin(t *testing.T, hook *fakeHook, address string, tune func(*plugin)) *bytes.Buffer {
	t.Helper()
	var logged bytes.Buffer
	var mu sync.Mutex
	p := newPlugin(hook.srv.URL, "hap_test", address, log.New(writerFunc(func(b []byte) (int, error) {
		mu.Lock()
		defer mu.Unlock()
		return logged.Write(b)
	}), "", 0))
	p.pause = func(ctx context.Context, d time.Duration) bool {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(time.Millisecond):
			return true
		}
	}
	if tune != nil {
		tune(p)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { p.run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	return &logged
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(b []byte) (int, error) { return f(b) }

// fakeFlareSolverr answers POST /v1 with reply, after checking the ask.
func fakeFlareSolverr(t *testing.T, status int, reply string, check func(map[string]any)) (*httptest.Server, *atomic.Int32) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/v1" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("FlareSolverr asked %s %s", r.Method, r.URL.Path)
		}
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		if check != nil {
			check(in)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, reply)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func solved(body string) string {
	b, _ := json.Marshal(map[string]any{"status": "ok", "message": "Challenge solved!", "solution": map[string]any{
		"url": pageURL, "status": 200, "response": body, "userAgent": agent,
		"cookies": []map[string]any{{"name": "cf_clearance", "value": cookieValue, "domain": ".site.example", "httpOnly": true}},
		"headers": map[string]string{}}})
	return string(b)
}

func errorText(t *testing.T, a map[string]json.RawMessage) string {
	t.Helper()
	if _, ok := a["data"]; ok {
		t.Fatalf("answer has data: %s", a["data"])
	}
	var s string
	if err := json.Unmarshal(a["error"], &s); err != nil || s == "" {
		t.Fatalf("answer has no error: %v", a)
	}
	return s
}

func noSecrets(t *testing.T, where, s string) {
	t.Helper()
	for _, secret := range []string{"KEY123SECRET", "apikey", "site.example", proxyPass, cookieValue} {
		if strings.Contains(s, secret) {
			t.Errorf("%s holds %q: %s", where, secret, s)
		}
	}
}

// TestOpensThePage: the page, the proxy and a wait inside the hook's 75
// seconds go to FlareSolverr, and its solution comes back in the hook's shape.
func TestOpensThePage(t *testing.T) {
	const feed = `<rss version="2.0"><channel><item><title>Dune</title></item></channel></rss>`
	fs, _ := fakeFlareSolverr(t, http.StatusOK, solved(feed), func(in map[string]any) {
		px, _ := in["proxy"].(map[string]any)
		if in["cmd"] != "request.get" || in["url"] != pageURL || in["maxTimeout"] != float64(60000) ||
			px["url"] != "socks5://vpn:1080" || px["username"] != "me" || px["password"] != proxyPass {
			t.Errorf("FlareSolverr got %v", in)
		}
	})
	hook := newFakeHook(t, openJob("j1"))
	logged := startPlugin(t, hook, fs.URL, nil)
	a := hook.wait(1)[0]
	var id string
	_ = json.Unmarshal(a["job"], &id)
	var got answer
	if err := json.Unmarshal(a["data"], &got); err != nil || id != "j1" {
		t.Fatalf("answer %v: %v", a, err)
	}
	if got.URL != pageURL || got.Status != 200 || got.Body != feed || got.UserAgent != agent ||
		len(got.Cookies) != 1 || got.Cookies[0] != (cookie{Name: "cf_clearance", Value: cookieValue}) {
		t.Errorf("answer %+v", got)
	}
	if _, ok := a["error"]; ok {
		t.Errorf("answer has an error: %s", a["error"])
	}
	// The browser's page is sent as it is, with < and > unescaped.
	if !bytes.Contains(a["data"], []byte(`"body":"<rss`)) {
		t.Errorf("body escaped: %s", a["data"])
	}
	noSecrets(t, "log", logged.String())
}

// TestNoProxyNoProxyField: a Source without a proxy sends none to FlareSolverr.
func TestNoProxyNoProxyField(t *testing.T) {
	fs, _ := fakeFlareSolverr(t, http.StatusOK, solved("{}"), func(in map[string]any) {
		if _, ok := in["proxy"]; ok {
			t.Errorf("proxy sent: %v", in["proxy"])
		}
	})
	p, _ := json.Marshal(map[string]any{"url": pageURL})
	hook := newFakeHook(t, job{ID: "j1", Op: "open", Payload: p})
	startPlugin(t, hook, fs.URL, nil)
	if _, ok := hook.wait(1)[0]["data"]; !ok {
		t.Fatal("no data")
	}
}

// TestFailures: each way FlareSolverr can fail is answered with a plain
// reason that names neither the page nor its key, and nothing secret is logged.
func TestFailures(t *testing.T) {
	big := strings.Repeat("x", bodyMax+1)
	for _, c := range []struct {
		name   string
		status int
		reply  string
		want   string
	}{
		{"error", http.StatusInternalServerError,
			`{"status":"error","message":"Error: Error solving the challenge. Timeout after 60.0 seconds. ` + pageURL + `","solution":null}`,
			"FlareSolverr could not pass the bot check: Error: Error solving the challenge. Timeout after 60.0 seconds."},
		{"still a bot check", http.StatusOK, solved(`<html><head><title>Just a moment...</title></head></html>`), "still on the bot check"},
		{"not FlareSolverr", http.StatusOK, `<html>router login</html>`, "not FlareSolverr"},
		{"too large", http.StatusOK, solved(big), "larger than 16 MB"},
	} {
		t.Run(c.name, func(t *testing.T) {
			fs, _ := fakeFlareSolverr(t, c.status, c.reply, nil)
			hook := newFakeHook(t, openJob("j1"))
			logged := startPlugin(t, hook, fs.URL, nil)
			msg := errorText(t, hook.wait(1)[0])
			if !strings.Contains(msg, c.want) {
				t.Errorf("error %q, want %q", msg, c.want)
			}
			noSecrets(t, "error", msg)
			noSecrets(t, "log", logged.String())
		})
	}
}

// TestTimesOut: FlareSolverr that takes too long is given up on, and the
// hook is told so.
func TestTimesOut(t *testing.T) {
	stop := make(chan struct{})
	fs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-stop:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(fs.Close)
	t.Cleanup(func() { close(stop) })
	hook := newFakeHook(t, openJob("j1"))
	logged := startPlugin(t, hook, fs.URL, func(p *plugin) { p.fs.callWait = 50 * time.Millisecond })
	msg := errorText(t, hook.wait(1)[0])
	if !strings.Contains(msg, "took too long") {
		t.Errorf("error %q", msg)
	}
	noSecrets(t, "log", logged.String())
}

// TestDownBacksOff: FlareSolverr that does not answer is asked once; jobs
// in the next while get the same answer without asking it again.
func TestDownBacksOff(t *testing.T) {
	var calls atomic.Int32
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			calls.Add(1)
			c.Close()
		}
	}()
	hook := newFakeHook(t, openJob("j1"), openJob("j2"), openJob("j3"))
	logged := startPlugin(t, hook, "http://"+ln.Addr().String(), func(p *plugin) { p.slots = make(chan struct{}, 1) })
	for _, a := range hook.wait(3) {
		if msg := errorText(t, a); !strings.Contains(msg, "FlareSolverr did not answer") {
			t.Errorf("error %q", msg)
		}
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("FlareSolverr asked %d times while down", n)
	}
	noSecrets(t, "log", logged.String())
}

// TestDownWindowGrows: each failure doubles the wait before FlareSolverr is
// asked again, up to a cap, and an answer clears it.
func TestDownWindowGrows(t *testing.T) {
	f := &flareSolverr{}
	n := time.Unix(1000, 0)
	f.now = func() time.Time { return n }
	var got []time.Duration
	for range 8 {
		f.wentDown()
		got = append(got, f.downUntil.Sub(n))
	}
	want := []time.Duration{10 * time.Second, 20 * time.Second, 40 * time.Second, 80 * time.Second, 2 * time.Minute, 2 * time.Minute, 2 * time.Minute, 2 * time.Minute}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("waits %v, want %v", got, want)
		}
	}
	f.answered()
	if f.isDown() {
		t.Error("still down after an answer")
	}
	f.wentDown()
	if f.downUntil.Sub(n) != 10*time.Second {
		t.Errorf("wait after an answer: %v", f.downUntil.Sub(n))
	}
}

// TestHookBackOff: when HolyArrs does not answer the poll, the plugin waits
// longer each time, up to a minute, and starts again at once after it answers.
func TestHookBackOff(t *testing.T) {
	var fails atomic.Int32
	fails.Store(9)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fails.Add(-1) >= 0 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	var mu sync.Mutex
	var waits []time.Duration
	var logged bytes.Buffer
	p := newPlugin(srv.URL, "hap_test", "", log.New(&logged, "", 0))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.pause = func(ctx context.Context, d time.Duration) bool {
		mu.Lock()
		defer mu.Unlock()
		waits = append(waits, d)
		if len(waits) >= 11 {
			cancel()
		}
		return ctx.Err() == nil
	}
	p.run(ctx)
	want := []time.Duration{1, 2, 4, 8, 16, 32, 60, 60, 60}
	if len(waits) < len(want)+1 {
		t.Fatalf("waits %v", waits)
	}
	for i, w := range want {
		if waits[i] != w*time.Second {
			t.Fatalf("waits %v", waits)
		}
	}
	// After HolyArrs answered 204 at once, the next poll waits only the short
	// guard against spinning, not the back-off.
	if waits[len(want)] != time.Second {
		t.Errorf("wait after an answer: %v", waits[len(want)])
	}
}

// TestHookRefused: a plugin without the sources.botcheck action is told 403;
// it waits the longest and says what to do, never spinning.
func TestHookRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)
	var logged bytes.Buffer
	p := newPlugin(srv.URL, "hap_test", "", log.New(&logged, "", 0))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var waits []time.Duration
	p.pause = func(ctx context.Context, d time.Duration) bool {
		waits = append(waits, d)
		if len(waits) == 2 {
			cancel()
		}
		return ctx.Err() == nil
	}
	p.run(ctx)
	if len(waits) != 2 || waits[0] != time.Minute || waits[1] != time.Minute {
		t.Errorf("waits %v", waits)
	}
	if !strings.Contains(logged.String(), "sources.botcheck") {
		t.Errorf("log %q", logged.String())
	}
}

// TestNoAddress: with no address set, each job is answered with what to do.
func TestNoAddress(t *testing.T) {
	hook := newFakeHook(t, openJob("j1"))
	startPlugin(t, hook, "", nil)
	if msg := errorText(t, hook.wait(1)[0]); !strings.Contains(msg, "address is not set") {
		t.Errorf("error %q", msg)
	}
}

// TestBadJobs: an op it does not know, or a job without a page, is answered
// with an error and never reaches FlareSolverr.
func TestBadJobs(t *testing.T) {
	fs, calls := fakeFlareSolverr(t, http.StatusOK, solved("{}"), nil)
	hook := newFakeHook(t, job{ID: "j1", Op: "close"}, job{ID: "j2", Op: "open", Payload: json.RawMessage(`{"url":""}`)},
		job{ID: "j3", Op: "open", Payload: json.RawMessage(`{"url":"file:///etc/passwd"}`)})
	startPlugin(t, hook, fs.URL, nil)
	for _, a := range hook.wait(3) {
		errorText(t, a)
	}
	if calls.Load() != 0 {
		t.Errorf("FlareSolverr asked %d times", calls.Load())
	}
}

func TestParseAddress(t *testing.T) {
	for in, want := range map[string]string{
		"http://flaresolverr:8191":       "http://flaresolverr:8191",
		"flaresolverr:8191":              "http://flaresolverr:8191",
		" http://fs.home.arpa:8191/ ":    "http://fs.home.arpa:8191",
		"http://fs.home.arpa:8191/v1":    "http://fs.home.arpa:8191",
		"https://fs.example.com":         "https://fs.example.com",
		"":                               "",
		"ftp://fs:8191":                  "!",
		"http://user:pass@fs:8191":       "!",
		"http://fs:8191/other":           "!",
		"http://fs:8191/v1?x=1":          "!",
		"http://":                        "!",
		"http://fs.home.arpa:8191#frag":  "!",
		"javascript:alert(1)":            "!",
		"http://fs.home.arpa:8191/v1/v1": "!",
	} {
		got, err := parseAddress(in)
		if want == "!" {
			if err == nil {
				t.Errorf("%q: took %q", in, got)
			}
			continue
		}
		if err != nil || got != want {
			t.Errorf("%q: %q, %v; want %q", in, got, err, want)
		}
	}
}

func TestClean(t *testing.T) {
	got := clean("Error at "+pageURL+" and http://other.example/x?apikey=1 failed", pageURL)
	noSecrets(t, "clean", got)
	if strings.Contains(got, "http") || !strings.Contains(got, "failed") {
		t.Errorf("got %q", got)
	}
	if n := len(clean(strings.Repeat("é", 400), "")); n > 200 {
		t.Errorf("clipped to %d bytes", n)
	}
}

func TestStillBotCheck(t *testing.T) {
	for body, want := range map[string]bool{
		`<html><head><title>Just a moment...</title></head></html>`:             true,
		`<HTML><title>DDoS-Guard</title></HTML>`:                                true,
		`<script>window._cf_chl_opt={}</script>`:                                true,
		`<rss><channel><title>Just a moment of silence</title></channel></rss>`: false,
		`{"results":[]}`: false,
	} {
		if got := stillBotCheck(body); got != want {
			t.Errorf("%q: %v", body, got)
		}
	}
}
