package api

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/auth"
	"github.com/HolyArrs/holyarrs/app/backend/internal/dlsync"
	"github.com/HolyArrs/holyarrs/app/backend/internal/live"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/store"
)

type liveUser struct {
	c    *http.Client
	csrf string
	id   int64
}

type liveSetup struct {
	srv  *httptest.Server
	hub  *live.Hub
	sy   *dlsync.Sync
	own  liveUser
	mem  liveUser
	mem2 liveUser
	st   *store.Store
}

func (x liveSetup) call(t *testing.T, u liveUser, method, path, body string, csrf bool) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, x.srv.URL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if csrf {
		req.Header.Set(auth.CSRFHeader, u.csrf)
	}
	c := u.c
	if c == nil {
		c = http.DefaultClient
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// setupLive signs in an owner and two members against the real API.
func setupLive(t *testing.T) liveSetup {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(dir, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	v, err := secrets.Open(context.Background(), s, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	hub := live.NewHub(live.Options{})
	sy := &dlsync.Sync{Store: &dlsync.Store{}, DB: s}
	x := liveSetup{hub: hub, sy: sy, st: s}
	x.srv = httptest.NewServer(New(Server{Store: s, Metadata: metadata.Lab(), Version: "test", Auth: auth.New(s, auth.Options{Vault: v}), Secrets: v,
		Live: hub, DLSync: sy}).Handler())
	t.Cleanup(x.srv.Close)
	newUser := func() liveUser {
		jar, _ := cookiejar.New(nil)
		return liveUser{c: &http.Client{Jar: jar}}
	}
	read := func(body string) (string, string) {
		var r struct {
			CSRF string `json:"csrf"`
			Path string `json:"path"`
		}
		_ = json.Unmarshal([]byte(body), &r)
		return r.CSRF, r.Path
	}
	me := func(u *liveUser) {
		_, b := x.call(t, *u, "GET", "/api/v1/auth/me", "", false)
		var r struct {
			ID int64 `json:"id"`
		}
		_ = json.Unmarshal([]byte(b), &r)
		u.id = r.ID
	}
	x.own = newUser()
	st, body := x.call(t, x.own, "POST", "/api/v1/auth/setup", `{"name":"luis","password":"a long test passphrase"}`, false)
	if st != 201 {
		t.Fatalf("setup %d %s", st, body)
	}
	x.own.csrf, _ = read(body)
	me(&x.own)
	join := func(name string) liveUser {
		st, body := x.call(t, x.own, "POST", "/api/v1/users/invites", `{}`, true)
		if st != 201 {
			t.Fatalf("invite %d %s", st, body)
		}
		_, link := read(body)
		u := newUser()
		st, body = x.call(t, u, "POST", "/api/v1/auth/invites/"+strings.TrimPrefix(link, "/invite/"), `{"name":"`+name+`","password":"a long test passphrase"}`, false)
		if st != 201 {
			t.Fatalf("join %d %s", st, body)
		}
		u.csrf, _ = read(body)
		me(&u)
		return u
	}
	x.mem = join("ana")
	x.mem2 = join("bea")
	if x.own.id == 0 || x.mem.id == 0 || x.mem2.id == 0 || x.mem.id == x.mem2.id {
		t.Fatalf("user ids %d %d %d", x.own.id, x.mem.id, x.mem2.id)
	}
	return x
}

type liveFrame struct {
	id, topic, data string
}

func (x liveSetup) stream(t *testing.T, ctx context.Context, u liveUser, last string) <-chan liveFrame {
	t.Helper()
	path := x.srv.URL + "/live"
	if last != "" {
		path += "?last=" + last
	}
	return x.streamWith(t, ctx, u.c, path)
}

// streamURL opens a stream at url without a signed-in client.
func (x liveSetup) streamURL(t *testing.T, ctx context.Context, url string) <-chan liveFrame {
	t.Helper()
	return x.streamWith(t, ctx, http.DefaultClient, url)
}

func (x liveSetup) streamWith(t *testing.T, ctx context.Context, c *http.Client, path string) <-chan liveFrame {
	t.Helper()
	req, _ := http.NewRequestWithContext(ctx, "GET", path, nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("live: %d", resp.StatusCode)
	}
	out := make(chan liveFrame, 64)
	go func() {
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		var f liveFrame
		for sc.Scan() {
			l := sc.Text()
			switch {
			case l == "":
				if f.data != "" {
					out <- f
				}
				f = liveFrame{}
			case strings.HasPrefix(l, "id: "):
				f.id = l[4:]
			case strings.HasPrefix(l, "data: "):
				var env struct {
					Topic string          `json:"topic"`
					Data  json.RawMessage `json:"data"`
				}
				_ = json.Unmarshal([]byte(l[6:]), &env)
				f.topic, f.data = env.Topic, string(env.Data)
			}
		}
	}()
	return out
}

func frames(ch <-chan liveFrame, wait time.Duration) []liveFrame {
	var out []liveFrame
	deadline := time.After(wait)
	for {
		select {
		case f := <-ch:
			out = append(out, f)
		case <-deadline:
			return out
		}
	}
}

// Members may open /live now, but receive only events for their own items,
// live and when resuming with ?last=; managers keep receiving everything.
// route: GET /live
func TestLiveMembersSeeOnlyTheirOwn(t *testing.T) {
	x := setupLive(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	start := x.hub.Publish("job", "start")
	x.hub.Publish("event", "manager only")
	x.hub.PublishTo("ask", "mine", live.User(x.mem.id))
	x.hub.PublishTo("ask", "theirs", live.User(x.mem2.id))
	x.hub.PublishDL("1", "their dl", live.User(x.mem2.id))

	mem := x.stream(t, ctx, x.mem, x.hub.EventID(start))
	own := x.stream(t, ctx, x.own, x.hub.EventID(start))
	for x.hub.Clients() != 2 {
		time.Sleep(5 * time.Millisecond)
	}
	x.hub.Publish("slot", "manager only live")
	x.hub.PublishTo("ask", "mine live", live.User(x.mem.id))
	x.hub.PublishDL("1", "their dl live", live.User(x.mem2.id))

	got := frames(mem, 400*time.Millisecond)
	if len(got) != 2 || got[0].data != `"mine"` || got[1].data != `"mine live"` {
		t.Fatalf("member got %+v", got)
	}
	if all := frames(own, 200*time.Millisecond); len(all) != 7 {
		t.Fatalf("owner got %d events, want 7: %+v", len(all), all)
	}
	// A member resuming from another boot (a restart) is told to reload.
	re := frames(x.stream(t, ctx, x.mem, "0123456789ab-5"), 200*time.Millisecond)
	if len(re) != 1 || re[0].topic != "resync" {
		t.Fatalf("restart resume %+v", re)
	}
	// Signed out: no stream.
	if st, _ := x.call(t, liveUser{}, "GET", "/live", "", false); st != 401 {
		t.Fatalf("signed out live %d", st)
	}
}

// route: POST /api/v1/live/interest
func TestLiveInterest(t *testing.T) {
	x := setupLive(t)
	now := time.Now()
	x.sy.Store.Now = func() time.Time { return now }
	body := `{"view":"downloads"}`
	if st, _ := x.call(t, liveUser{}, "POST", "/api/v1/live/interest", body, false); st != 401 {
		t.Fatalf("signed out %d", st)
	}
	if st, _ := x.call(t, x.mem, "POST", "/api/v1/live/interest", body, false); st != 403 {
		t.Fatalf("no CSRF token %d", st)
	}
	// Another site's page cannot send it either (foreign Origin).
	req, _ := http.NewRequest("POST", x.srv.URL+"/api/v1/live/interest", strings.NewReader(body))
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(auth.CSRFHeader, x.mem.csrf)
	if resp, err := x.mem.c.Do(req); err != nil || resp.StatusCode != 403 {
		t.Fatalf("cross-origin: %v %v", resp, err)
	} else {
		resp.Body.Close()
	}
	if x.sy.Store.Viewer() {
		t.Fatal("refused calls counted")
	}
	if st, b := x.call(t, x.mem, "POST", "/api/v1/live/interest", `{"view":"titles"}`, true); st != 400 {
		t.Fatalf("unknown view %d %s", st, b)
	}
	if st, b := x.call(t, x.mem, "POST", "/api/v1/live/interest", body, true); st != 200 || !strings.Contains(b, `"ttl_seconds":60`) {
		t.Fatalf("member interest %d %s", st, b)
	}
	if !x.sy.Store.Viewer() {
		t.Fatal("interest did not count as a viewer")
	}
	// A second call within 10 s is a no-op; another session still counts.
	if st, _ := x.call(t, x.mem, "POST", "/api/v1/live/interest", body, true); st != 204 {
		t.Fatalf("second call %d, want 204", st)
	}
	if st, _ := x.call(t, x.own, "POST", "/api/v1/live/interest", body, true); st != 200 {
		t.Fatalf("owner call %d", st)
	}
	// After 60 s without a call the reader's viewer pace ends.
	now = now.Add(61 * time.Second)
	if x.sy.Store.Viewer() {
		t.Fatal("interest outlived its 60 s")
	}
}
