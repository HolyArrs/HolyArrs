package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	setupmod "github.com/HolyArrs/holyarrs/app/backend/internal/setup"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

type ctlFix struct {
	srv     *httptest.Server
	s       *database.Store
	pc      *parts.Control
	reg     *jobs.Registry
	api     *handlers
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
	srvOpts := services{Store: s, Queue: q, Metadata: metadata.Lab(), Version: "test", Parts: f.pc, States: f.reg, SafeMode: safe,
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
	f.api = buildHandlers(srvOpts)
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

// route: GET /api/v1/parts
// route: POST /api/v1/parts/{part}/on
// route: POST /api/v1/parts/{part}/off
// route: POST /api/v1/parts/{part}/restart
func TestPartSwitchesOverHTTP(t *testing.T) {
	f := newCtl(t, false)
	u := f.srv.URL + "/api/v1/"
	var got struct {
		Parts    []setupmod.PartView    `json:"parts"`
		Screens  map[string]parts.Owner `json:"screens"`
		SafeMode bool                   `json:"safe_mode"`
	}
	decodeJSON(t, do(t, "GET", u+"parts", "", 200), &got)
	if len(got.Parts) != 5 || !got.Parts[2].On || got.Parts[2].State != "running" || got.Screens["/settings/music"] != parts.Of(parts.Music) {
		t.Fatalf("parts: %+v", got)
	}
	do(t, "GET", u+"settings/music", "", 200)

	confirmTwice(t, u+"parts/music/off", 200)
	if b := do(t, "GET", u+"settings/music", "", 409); !strings.Contains(string(b), "this part is off") || !strings.Contains(string(b), "Music is off") {
		t.Fatalf("music route while off: %s", b)
	}
	decodeJSON(t, do(t, "GET", u+"parts", "", 200), &got)
	if got.Parts[2].On || got.Parts[2].State != "off" {
		t.Fatalf("music after off: %+v", got.Parts[2])
	}
	do(t, "GET", u+"settings/shows", "", 200) // other parts go on
	do(t, "GET", u+"health", "", 200)         // and the core

	do(t, "POST", u+"parts/music/on", "", 200)
	do(t, "GET", u+"settings/music", "", 200)

	// A confirm id works once, only for its own control, and for 60 s.
	var ask struct{ Confirm string }
	decodeJSON(t, do(t, "POST", u+"parts/shows/off", "{}", 202), &ask)
	do(t, "POST", u+"parts/music/off", `{"confirm":"`+ask.Confirm+`"}`, 409)
	defer func() { webserver.ConfirmNow = time.Now }()
	webserver.ConfirmNow = func() time.Time { return time.Now().Add(61 * time.Second) }
	do(t, "POST", u+"parts/shows/off", `{"confirm":"`+ask.Confirm+`"}`, 409)
	webserver.ConfirmNow = time.Now
	do(t, "POST", u+"parts/shows/off", `{"confirm":"nope"}`, 409)
	do(t, "POST", u+"parts/shows/off", `not json`, 400)
	if !f.pc.On(parts.Shows) {
		t.Fatal("shows went off without a good confirm")
	}

	// Dependencies are refused with the reason.
	confirmTwice(t, u+"parts/movies/off", 200)
	if b := confirmTwice(t, u+"parts/shows/off", 409); !strings.Contains(string(b), "Turn Subtitles off first") {
		t.Fatalf("shows off with subtitles on: %s", b)
	}
	do(t, "GET", u+"settings/subtitles", "", 200)
	do(t, "POST", u+"parts/movies/on", "", 200)

	b := confirmTwice(t, u+"parts/movies/restart", 200)
	if !strings.Contains(string(b), "Movies restarted") {
		t.Fatalf("restart: %s", b)
	}
	do(t, "POST", u+"parts/books/on", "", 404)

	var kinds []string
	for _, e := range f.events(t) {
		kinds = append(kinds, e.Data["action"].(string)+":"+e.Data["target"].(string))
		if e.Actor.ID != 1 || !strings.HasPrefix(e.Summary, "The owner ") || e.At.IsZero() {
			t.Errorf("event: %+v", e)
		}
	}
	want := "part_restart:movies part_on:movies part_off:movies part_on:music part_off:music"
	if strings.Join(kinds, " ") != want {
		t.Fatalf("events %v, want %s", kinds, want)
	}
}

// A title route of a part that is off says so; the title is hidden from the
// Library list but kept, and comes back when the part is on.
// route: GET /api/v1/titles/{id}
func TestTitleOfAnOffPart(t *testing.T) {
	f := newCtl(t, false)
	var id int64
	err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		ti := &mediafiles.Title{Kind: "movie", GateDriver: "none", Name: "Salt Choir", SortName: "salt choir", Aliases: []string{}, SourceIDs: map[string]string{},
			MetadataSource: "local", NewPartsPolicy: "all", ReleaseGate: "released", MusicTypes: []string{}, WhyAdded: "by_hand", Artwork: []string{}, Status: "released", State: "active"}
		var err error
		id, err = database.Insert(tx, "titles", ti, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	u := f.srv.URL + "/api/v1/"
	do(t, "GET", u+"titles/"+itoa(id), "", 200)
	confirmTwice(t, u+"parts/movies/off", 200)
	do(t, "GET", u+"titles/"+itoa(id), "", 409)
	do(t, "GET", u+"titles/999999", "", 404)
	if b := do(t, "GET", u+"titles", "", 200); strings.Contains(string(b), "Salt Choir") {
		t.Fatalf("an off part's title is listed: %s", b)
	}
	do(t, "POST", u+"parts/movies/on", "", 200)
	if b := do(t, "GET", u+"titles", "", 200); !strings.Contains(string(b), "Salt Choir") {
		t.Fatalf("title gone after on: %s", b)
	}
}

// No one can ask for a title of a part that is off.
func TestAskRefusedWhenItsPartIsOff(t *testing.T) {
	f := newCtl(t, false)
	u := f.srv.URL + "/api/v1/"
	confirmTwice(t, u+"parts/subtitles/off", 200)
	confirmTwice(t, u+"parts/shows/off", 200)
	if b := do(t, "POST", u+"asks", `{"kind":"show","id":"tvmaze:1"}`, 409); !strings.Contains(string(b), "TV shows is off") {
		t.Fatalf("show ask: %s", b)
	}
	confirmTwice(t, u+"parts/movies/off", 200)
	do(t, "POST", u+"asks", `{"id":"tmdb:1"}`, 409)
	// Requests itself off: every ask route says so.
	confirmTwice(t, u+"parts/wishlist/off", 200)
	do(t, "GET", u+"asks", "", 409)
}

// route: POST /api/v1/stages/{name}/pause
// route: POST /api/v1/stages/{name}/resume
// route: POST /api/v1/stages/{name}/restart
func TestStageControls(t *testing.T) {
	f := newCtl(t, false)
	u := f.srv.URL + "/api/v1/stages/"
	var out struct {
		State jobs.Status `json:"state"`
	}
	decodeJSON(t, do(t, "POST", u+"backups/pause", "", 200), &out)
	if out.State.State != jobs.Off || out.State.Reason != "Paused by The owner." {
		t.Fatalf("paused: %+v", out.State)
	}
	do(t, "POST", u+"backups/resume", "", 200)
	do(t, "POST", u+"backups/restart", "", 200)
	do(t, "POST", u+"nope/pause", "", 404)
	confirmTwice(t, f.srv.URL+"/api/v1/parts/movies/off", 200)
	do(t, "POST", u+"gates/resume", "", 409)
	do(t, "POST", u+"gates/restart", "", 409)
	if n := len(f.events(t)); n != 4 { // pause, resume, restart, part off
		t.Fatalf("%d events", n)
	}
}

// route: POST /api/v1/links/{kind}/{id}/restart
func TestLinkRestart(t *testing.T) {
	f := newCtl(t, false)
	var dl, src int64
	err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		d := &domain.Downloader{Name: "Q", Adapter: "qbittorrent", Protocol: "torrent", URL: "http://127.0.0.1:9", Category: "holyarrs", Priority: 1,
			PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok", AllowLocal: true}
		var err error
		if dl, err = database.Insert(tx, "downloaders", d, domain.System); err != nil {
			return err
		}
		x := &domain.Source{Name: "Lab", Protocol: "torrent", Format: "torznab", URL: "http://127.0.0.1:9", Kinds: []string{}, Categories: map[string][]int{},
			State: "enabled", Extra: map[string]string{}, AllowLocal: true}
		src, err = database.Insert(tx, "sources", x, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	u := f.srv.URL + "/api/v1/links/"
	for _, path := range []string{"downloader/" + itoa(dl), "source/" + itoa(src)} {
		var res struct {
			OK   bool   `json:"ok"`
			Text string `json:"text"`
		}
		decodeJSON(t, do(t, "POST", u+path+"/restart", "", 200), &res)
		if res.OK || res.Text == "" {
			t.Fatalf("%s: %+v", path, res)
		}
	}
	do(t, "POST", u+"subtitle_site/999/restart", "", 404) // no Event: the id is checked first
	do(t, "POST", u+"media_server/1/restart", "", 503)    // no media servers here
	do(t, "POST", u+"printer/1/restart", "", 404)
	es := f.events(t)
	if len(es) != 2 || es[len(es)-1].Data["target"] != "downloader:"+itoa(dl) {
		t.Fatalf("events: %+v", es)
	}
}

// Whole-app restart, restart in safe mode and shut down ask for a confirm,
// write an Event, answer with what Docker's restart policy does, and then
// ask the process to exit (a hook here records it).
// route: POST /api/v1/app/restart
// route: POST /api/v1/app/safe-mode
// route: POST /api/v1/app/shutdown
func TestAppExitControls(t *testing.T) {
	f := newCtl(t, false)
	u := f.srv.URL + "/api/v1/app/"
	for _, c := range []struct{ path, how string }{{"restart", "restart"}, {"safe-mode", "safe_mode"}, {"shutdown", "shutdown"}} {
		before := len(f.exited())
		var ask struct{ Confirm string }
		decodeJSON(t, do(t, "POST", u+c.path, "", 202), &ask)
		if len(f.exited()) != before {
			t.Fatalf("%s exited on the first step", c.path)
		}
		b := do(t, "POST", u+c.path, `{"confirm":"`+ask.Confirm+`"}`, 202)
		if !strings.Contains(string(b), "restart policy") || !strings.Contains(string(b), `\"no\"`) {
			t.Fatalf("%s: %s", c.path, b)
		}
		if ex := f.exited(); len(ex) != before+1 || ex[before] != c.how {
			t.Fatalf("exits %v", ex)
		}
	}
	if n := len(f.events(t)); n != 3 {
		t.Fatalf("%d events", n)
	}
	// Without an exit hook the controls are refused.
	s := plainServer(t)
	do(t, "POST", s.URL+"/api/v1/app/restart", "", 503)
}

// The health states list the parts.
func TestHealthStatesListParts(t *testing.T) {
	f := newCtl(t, false)
	confirmTwice(t, f.srv.URL+"/api/v1/parts/music/off", 200)
	var got struct {
		States []jobs.Status `json:"states"`
	}
	decodeJSON(t, do(t, "GET", f.srv.URL+"/api/v1/health/states", "", 200), &got)
	seen := map[string]jobs.State{}
	for _, s := range got.States {
		if s.Kind == "part" {
			seen[s.Key] = s.State
		}
	}
	if len(seen) != 5 || seen["part:music"] != jobs.Off || seen["part:movies"] != jobs.Running {
		t.Fatalf("parts in health states: %v", seen)
	}
}

// Only the owner and admins use the controls; an API key needs the control
// scope for them, and a control-only key reaches nothing else; a key cannot
// make a key with a scope it does not hold.
func TestControlsNeedRoleAndScope(t *testing.T) {
	f := newCtl(t, true)
	call := func(c *http.Client, csrf, key, method, path, body string) (int, string) {
		req, _ := http.NewRequest(method, f.srv.URL+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if csrf != "" {
			req.Header.Set(accounts.CSRFHeader, csrf)
		}
		if key != "" {
			req.Header.Set(accounts.APIKeyHeader, key)
		}
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	client := func() *http.Client { jar, _ := cookiejar.New(nil); return &http.Client{Jar: jar} }
	field := func(body, name string) string {
		var v map[string]any
		_ = json.Unmarshal([]byte(body), &v)
		s, _ := v[name].(string)
		return s
	}
	own := client()
	st, body := call(own, "", "", "POST", "/api/v1/auth/setup", `{"name":"luis","password":"a long test passphrase"}`)
	if st != 201 {
		t.Fatalf("setup %d %s", st, body)
	}
	csrf := field(body, "csrf")
	st, body = call(own, csrf, "", "POST", "/api/v1/users/invites", `{}`)
	if st != 201 {
		t.Fatalf("invite %d %s", st, body)
	}
	mem := client()
	st, body = call(mem, "", "", "POST", "/api/v1/auth/invites/"+strings.TrimPrefix(field(body, "path"), "/invite/"), `{"name":"ana","password":"a long test passphrase"}`)
	if st != 201 {
		t.Fatalf("join %d %s", st, body)
	}
	memCSRF := field(body, "csrf")
	if st, body := call(mem, memCSRF, "", "POST", "/api/v1/stages/backups/pause", ""); st != 403 {
		t.Fatalf("member pause: %d %s", st, body)
	}
	if st, _ := call(mem, memCSRF, "", "GET", "/api/v1/parts", ""); st != 200 {
		t.Fatalf("member parts list: %d", st)
	}
	if st, body := call(own, csrf, "", "POST", "/api/v1/stages/backups/pause", ""); st != 200 {
		t.Fatalf("owner pause: %d %s", st, body)
	}

	st, body = call(own, csrf, "", "POST", "/api/v1/auth/keys", `{"label":"all"}`)
	if st != 201 || !strings.Contains(body, `"scopes":["all"]`) {
		t.Fatalf("key: %d %s", st, body)
	}
	allKey := field(body, "key")
	st, body = call(own, csrf, "", "POST", "/api/v1/auth/keys", `{"label":"ctl","scopes":["control"]}`)
	if st != 201 {
		t.Fatalf("control key: %d %s", st, body)
	}
	ctlKey := field(body, "key")
	if st, _ := call(own, csrf, "", "POST", "/api/v1/auth/keys", `{"label":"x","scopes":["root"]}`); st != 400 {
		t.Fatalf("unknown scope: %d", st)
	}

	plain := &http.Client{}
	if st, body := call(plain, "", allKey, "POST", "/api/v1/stages/backups/resume", ""); st != 403 || !strings.Contains(body, "scope") {
		t.Fatalf("all key on a control: %d %s", st, body)
	}
	if st, _ := call(plain, "", allKey, "GET", "/api/v1/titles", ""); st != 200 {
		t.Fatalf("all key on titles: %d", st)
	}
	if st, body := call(plain, "", ctlKey, "POST", "/api/v1/stages/backups/resume", ""); st != 200 {
		t.Fatalf("control key on a control: %d %s", st, body)
	}
	if st, _ := call(plain, "", ctlKey, "GET", "/api/v1/titles", ""); st != 403 {
		t.Fatalf("control key on titles: %d", st)
	}
	if st, _ := call(plain, "", allKey, "POST", "/api/v1/auth/keys", `{"label":"up","scopes":["control"]}`); st != 403 {
		t.Fatalf("an all key made a control key: %d", st)
	}
	// 3.7.11: read, write and admin keys reach only their own routes, and
	// a key cannot make a key with more than it holds.
	st, body = call(own, csrf, "", "POST", "/api/v1/auth/keys", `{"label":"r","scopes":["read"]}`)
	if st != 201 {
		t.Fatalf("read key: %d %s", st, body)
	}
	readKey := field(body, "key")
	st, body = call(own, csrf, "", "POST", "/api/v1/auth/keys", `{"label":"adm","scopes":["admin"]}`)
	if st != 201 {
		t.Fatalf("admin key: %d %s", st, body)
	}
	admKey := field(body, "key")
	if st, _ := call(plain, "", readKey, "GET", "/api/v1/titles", ""); st != 200 {
		t.Fatalf("read key on titles: %d", st)
	}
	if st, body := call(plain, "", readKey, "POST", "/api/v1/asks", `{"id":"tmdb:1"}`); st != 403 || !strings.Contains(body, "write scope") {
		t.Fatalf("read key on a write: %d %s", st, body)
	}
	if st, body := call(plain, "", readKey, "GET", "/api/v1/users", ""); st != 403 || !strings.Contains(body, "admin scope") {
		t.Fatalf("read key on people: %d %s", st, body)
	}
	if st, _ := call(plain, "", readKey, "POST", "/api/v1/auth/keys", `{"label":"up","scopes":["all"]}`); st != 403 {
		t.Fatalf("a read key made a key: %d", st)
	}
	if st, _ := call(plain, "", admKey, "GET", "/api/v1/users", ""); st != 200 {
		t.Fatalf("admin key on people: %d", st)
	}
	if st, _ := call(plain, "", admKey, "GET", "/api/v1/titles", ""); st != 403 {
		t.Fatalf("admin key on titles: %d", st)
	}
	if st, body := call(plain, "", admKey, "POST", "/api/v1/auth/keys", `{"label":"up","scopes":["write"]}`); st != 403 || !strings.Contains(body, "does not hold") {
		t.Fatalf("an admin key made a write key: %d %s", st, body)
	}
	if st, body := call(plain, "", admKey, "POST", "/api/v1/auth/keys", `{"label":"same","scopes":["admin"]}`); st != 201 {
		t.Fatalf("an admin key made an admin key: %d %s", st, body)
	}
	if st, body := call(plain, "", allKey, "POST", "/api/v1/auth/keys", `{"label":"less","scopes":["read"]}`); st != 201 {
		t.Fatalf("an all key made a read key: %d %s", st, body)
	}
	st, body = call(own, csrf, "", "GET", "/api/v1/auth/keys", "")
	if st != 200 || !strings.Contains(body, `"scopes":["control"]`) {
		t.Fatalf("key list: %d %s", st, body)
	}
	// A member's ask waits while its part is off: it cannot be approved.
	st, body = call(mem, memCSRF, "", "POST", "/api/v1/asks", `{"id":"tmdb:693134"}`)
	if st != 201 {
		t.Fatalf("member ask: %d %s", st, body)
	}
	var made struct {
		Ask struct {
			ID    int64  `json:"id"`
			State string `json:"state"`
		} `json:"ask"`
	}
	_ = json.Unmarshal([]byte(body), &made)
	if made.Ask.ID == 0 || made.Ask.State != "asked" {
		t.Fatalf("ask: %s", body)
	}
	if st, body := call(plain, "", ctlKey, "POST", "/api/v1/parts/movies/off", `{}`); st != 202 {
		t.Fatalf("ask to turn movies off: %d %s", st, body)
	} else if st, body := call(plain, "", ctlKey, "POST", "/api/v1/parts/movies/off", `{"confirm":"`+field(body, "confirm")+`"}`); st != 200 {
		t.Fatalf("movies off: %d %s", st, body)
	}
	if st, body := call(own, csrf, "", "POST", "/api/v1/asks/"+itoa(made.Ask.ID)+"/approve", ""); st != 409 || !strings.Contains(body, "Movies is off") {
		t.Fatalf("approve while movies is off: %d %s", st, body)
	}
	if st, _ := call(mem, memCSRF, "", "POST", "/api/v1/asks", `{"id":"tmdb:693135"}`); st != 409 {
		t.Fatalf("member asks for a movie while movies is off: %d", st)
	}
	es := f.events(t)
	if len(es) != 3 || es[0].Data["action"] != "part_off" {
		t.Fatalf("events: %+v", es)
	}
	es = es[1:]
	if len(es) != 2 || es[0].Data["via"] != "api_key" || es[1].Data["via"] != "session" || !strings.HasPrefix(es[1].Summary, "luis paused") {
		t.Fatalf("events: %+v", es)
	}
}

// TestEveryRouteHasAPart reads every route the program registers (this
// package, auth and backup) and checks the parts table gives it an owner:
// a part, the media parts together, the title's kind, or the core. A row
// for a route that is gone fails too.
func TestEveryRouteHasAPart(t *testing.T) {
	re := regexp.MustCompile(`m(?:ux)?\.Handle(?:Func)?\("([A-Z]+ [^"]*)"`)
	seen := map[string]bool{}
	for _, dir := range routePackages {
		files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") || (dir == "../../internal/arrapi" && filepath.Base(f) != "routes_settings.go") || (dir == "." && filepath.Base(f) == "starting.go") {
				continue
			}
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			for _, m := range re.FindAllStringSubmatch(string(b), -1) {
				seen[m[1]] = true
				if _, ok := parts.Routes()[m[1]]; !ok {
					t.Errorf("%s: route %q belongs to no part and is not marked core in internal/core/parts/table.go", f, m[1])
				}
			}
		}
	}
	if len(seen) < 150 {
		t.Fatalf("found only %d routes; the pattern no longer matches the code", len(seen))
	}
	for r := range parts.Routes() {
		if !seen[r] {
			t.Errorf("internal/core/parts/table.go lists %q, but no such route is registered", r)
		}
	}
}

// plainServer serves the API with no exit hook and no sign-in.
func plainServer(t *testing.T) *httptest.Server {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	srv := httptest.NewServer(buildHandlers(services{Store: s, Metadata: metadata.Lab(), Version: "test"}).Handler())
	t.Cleanup(srv.Close)
	return srv
}
