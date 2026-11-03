package api

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
	"strings"
	"sync"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/asks"
	"github.com/CoderLuii/holyarrs/internal/auth"
	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/jobs"
	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/live"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/secrets"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// signedIn is one person's browser: cookies and the CSRF token.
type signedIn struct {
	t    *testing.T
	base string
	c    *http.Client
	csrf string
	id   int64
}

func (s *signedIn) call(method, path, body string, want int) []byte {
	s.t.Helper()
	req, _ := http.NewRequest(method, s.base+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(auth.CSRFHeader, s.csrf)
	resp, err := s.c.Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != want {
		s.t.Fatalf("%s %s: %d, want %d: %s", method, path, resp.StatusCode, want, b)
	}
	return b
}

type askRig struct {
	srv   *httptest.Server
	st    *store.Store
	asks  *asks.Service
	owner *signedIn
	ana   *signedIn
	bo    *signedIn
	api   *API
}

// newAskRig runs the real API with sign-in: the owner luis and the members
// ana and bo (members may ask, nothing more).
func newAskRig(t *testing.T) *askRig {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(dir, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	if err := st.Write(ctx, insertFolder(t)); err != nil {
		t.Fatal(err)
	}
	v, err := secrets.Open(ctx, st, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	au := auth.New(st, auth.Options{Vault: v})
	md := &metadata.Static{Movies: []metadata.Movie{metadata.Lab().Movies[0],
		{Name: "Paper Harbour", Year: 2023, IDs: map[string]string{"tmdb": "900002"}},
		{Name: "Quiet Orchard", Year: 2022, IDs: map[string]string{"tmdb": "900003"}}}}
	q := jobs.New(st, jobs.Options{Workers: 1})
	a := New(Server{Store: st, Queue: q, Metadata: md, Version: "test", Auth: au, Secrets: v, Live: live.NewHub(live.Options{})})
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	r := &askRig{srv: srv, st: st, asks: a.s.Asks, api: a}

	browser := func() *signedIn {
		jar, _ := cookiejar.New(nil)
		return &signedIn{t: t, base: srv.URL, c: &http.Client{Jar: jar}}
	}
	read := func(b []byte) (string, string) {
		var x struct {
			CSRF string `json:"csrf"`
			Path string `json:"path"`
		}
		_ = json.Unmarshal(b, &x)
		return x.CSRF, x.Path
	}
	code, _ := os.ReadFile(filepath.Join(dir, auth.SetupCodeFile))
	r.owner = browser()
	r.owner.csrf, _ = read(r.owner.call("POST", "/api/v1/auth/setup", `{"name":"luis","password":"a long test passphrase","code":"`+strings.TrimSpace(string(code))+`"}`, 201))
	join := func(name string) *signedIn {
		_, link := read(r.owner.call("POST", "/api/v1/users/invites", `{}`, 201))
		b := browser()
		b.csrf, _ = read(b.call("POST", "/api/v1/auth/invites/"+strings.TrimPrefix(link, "/invite/"), `{"name":"`+name+`","password":"a long test passphrase"}`, 201))
		var me struct {
			ID int64 `json:"id"`
		}
		_ = json.Unmarshal(b.call("GET", "/api/v1/auth/me", "", 200), &me)
		b.id = me.ID
		return b
	}
	r.ana, r.bo = join("ana"), join("bo")
	return r
}

func insertFolder(t *testing.T) func(tx *sql.Tx) error {
	dir := t.TempDir()
	return func(tx *sql.Tx) error {
		_, err := store.Insert(tx, "library_folders", &domain.LibraryFolder{Path: dir, Kind: "movie", Label: "Movies", Health: "ok", SameDiskAs: []int64{}}, domain.System)
		return err
	}
}

func askOf(t *testing.T, b []byte) asks.View {
	t.Helper()
	var x struct {
		Ask asks.View `json:"ask"`
	}
	if err := json.Unmarshal(b, &x); err != nil {
		t.Fatal(err)
	}
	return x.Ask
}

// route: POST /api/v1/asks
// route: GET /api/v1/asks
// route: GET /api/v1/asks/{id}
// route: POST /api/v1/asks/{id}/approve
// route: GET /api/v1/messages
// route: POST /api/v1/messages/read
func TestAskJoinApproveOverHTTP(t *testing.T) {
	r := newAskRig(t)
	a := askOf(t, r.ana.call("POST", "/api/v1/asks", `{"id":"tmdb:693134"}`, 201))
	if a.State != "asked" || a.AskedBy.Name != "ana" || !a.Mine {
		t.Fatalf("ask %+v", a)
	}
	r.ana.call("POST", "/api/v1/asks", `{"id":"imdb:tt1"}`, 400)
	r.ana.call("POST", "/api/v1/asks", `{"id":"tmdb:1"}`, 404)

	// bo joins; the lookup shows him the open ask.
	var look struct {
		Movies []LookupView `json:"movies"`
	}
	_ = json.Unmarshal(r.bo.call("GET", "/api/v1/titles/lookup?term=tmdb:693134", "", 200), &look)
	if len(look.Movies) != 1 || look.Movies[0].Ask == nil || look.Movies[0].Ask.Mine || look.Movies[0].Ask.State != "asked" {
		t.Fatalf("lookup %+v", look)
	}
	// A member sees only themself among the askers (Asked by privacy).
	b := r.bo.call("POST", "/api/v1/asks", `{"id":"tmdb:693134"}`, 200)
	if !strings.Contains(string(b), `"joined":true`) || len(askOf(t, b).People) != 1 {
		t.Fatalf("join %s", b)
	}

	// Members see only their own asks, and may not decide.
	r.bo.call("POST", "/api/v1/asks", `{"id":"tmdb:900003"}`, 201)
	var list struct {
		Asks []asks.View `json:"asks"`
	}
	_ = json.Unmarshal(r.ana.call("GET", "/api/v1/asks", "", 200), &list)
	if len(list.Asks) != 1 {
		t.Fatalf("ana sees %+v", list.Asks)
	}
	_ = json.Unmarshal(r.owner.call("GET", "/api/v1/asks?state=asked", "", 200), &list)
	if len(list.Asks) != 2 {
		t.Fatalf("owner sees %+v", list.Asks)
	}
	r.owner.call("GET", "/api/v1/asks?state=nope", "", 400)
	other := list.Asks[0]
	if other.Name != "Quiet Orchard" {
		other = list.Asks[1]
	}
	r.ana.call("GET", "/api/v1/asks/"+itoa(other.ID), "", 404)
	r.ana.call("POST", "/api/v1/asks/"+itoa(a.ID)+"/approve", `{}`, 403)
	r.bo.call("GET", "/api/v1/asks/"+itoa(a.ID), "", 200)

	var msgs struct {
		Unread  int  `json:"unread"`
		Waiting *int `json:"waiting"`
	}
	_ = json.Unmarshal(r.owner.call("GET", "/api/v1/messages", "", 200), &msgs)
	if msgs.Waiting == nil || *msgs.Waiting != 2 {
		t.Fatalf("owner messages %+v", msgs)
	}

	got := askOf(t, r.owner.call("POST", "/api/v1/asks/"+itoa(a.ID)+"/approve", `{"note":"popcorn"}`, 200))
	if got.State != "approved" || got.DecidedBy == nil || got.DecidedBy.Name != "luis" || got.TitleID == nil {
		t.Fatalf("approved %+v", got)
	}
	r.owner.call("POST", "/api/v1/asks/"+itoa(a.ID)+"/approve", ``, 409)
	r.owner.call("POST", "/api/v1/asks/999/approve", ``, 404)

	// The Movie page names who asked and who approved; a member sees only
	// themself among the askers (Asked by privacy).
	var tv TitleView
	_ = json.Unmarshal(r.ana.call("GET", "/api/v1/titles/"+itoa(*got.TitleID), "", 200), &tv)
	if len(tv.Asks) != 1 || len(tv.Asks[0].People) != 1 || tv.Asks[0].DecidedBy.Name != "luis" || tv.WhyAdded != "ask" {
		t.Fatalf("title asks %+v", tv.Asks)
	}
	msgs.Waiting = nil
	// A member who did not ask does not see who did.
	orchard := askOf(t, r.owner.call("POST", "/api/v1/asks/"+itoa(other.ID)+"/approve", ``, 200))
	var otv TitleView
	_ = json.Unmarshal(r.ana.call("GET", "/api/v1/titles/"+itoa(*orchard.TitleID), "", 200), &otv)
	if len(otv.Asks) != 0 {
		t.Fatalf("ana sees bo's ask: %+v", otv.Asks)
	}
	_ = json.Unmarshal(r.bo.call("GET", "/api/v1/titles/"+itoa(*orchard.TitleID), "", 200), &otv)
	if len(otv.Asks) != 1 || !otv.Asks[0].Mine {
		t.Fatalf("bo's own ask: %+v", otv.Asks)
	}
	_ = json.Unmarshal(r.ana.call("GET", "/api/v1/messages", "", 200), &msgs)
	if msgs.Unread != 1 || msgs.Waiting != nil {
		t.Fatalf("ana messages %+v", msgs)
	}
	r.ana.call("POST", "/api/v1/messages/read", "", 204)
	_ = json.Unmarshal(r.ana.call("GET", "/api/v1/messages", "", 200), &msgs)
	if msgs.Unread != 0 {
		t.Fatalf("still unread %+v", msgs)
	}
	// Someone else asking for it now is told it is in the Library.
	b = r.owner.call("POST", "/api/v1/asks", `{"id":"tmdb:693134"}`, 200) // the approved ask is still open: luis joins
	if !strings.Contains(string(b), `"joined":true`) {
		t.Fatalf("join approved %s", b)
	}
	if _, _, err := r.asks.Library.Add(context.Background(), metadata.Movie{Name: "Quiet Orchard", Year: 2022, IDs: map[string]string{"tmdb": "900005"}},
		library.AddOptions{}, domain.System); err != nil {
		t.Fatal(err)
	}
	b = r.ana.call("POST", "/api/v1/asks", `{"id":"tmdb:900005"}`, 409)
	if !strings.Contains(string(b), "already in the Library") || !strings.Contains(string(b), `"title_id"`) {
		t.Fatalf("in library %s", b)
	}
	r.asks.Wait()
}

// route: POST /api/v1/asks/{id}/turn-down
// route: DELETE /api/v1/asks/{id}
// route: GET /api/v1/asks/limit
// route: GET /api/v1/settings/asks
// route: PUT /api/v1/settings/asks
// route: GET /api/v1/ask-limits
// route: PUT /api/v1/ask-limits/{id}
func TestLimitTurnDownAndTakeBackOverHTTP(t *testing.T) {
	r := newAskRig(t)
	var set asks.Settings
	_ = json.Unmarshal(r.owner.call("GET", "/api/v1/settings/asks", "", 200), &set)
	if set.MoviesPerWeek != asks.DefaultPerWeek || set.ShowsPerWeek != asks.DefaultPerWeek || set.MusicPerWeek != asks.DefaultPerWeek || !set.SearchOnApprove {
		t.Fatalf("defaults %+v", set)
	}
	r.owner.call("PUT", "/api/v1/settings/asks", `{"movies_per_week":1,"search_on_approve":false}`, 200)
	r.owner.call("PUT", "/api/v1/settings/asks", `{"movies_per_week":-2}`, 400)
	r.owner.call("PUT", "/api/v1/settings/asks", `{"music_per_week":1001}`, 400)
	r.owner.call("PUT", "/api/v1/settings/asks", `{"movies_per_week":1,"library_folder_id":99}`, 409)
	r.ana.call("GET", "/api/v1/settings/asks", "", 403)

	a := askOf(t, r.ana.call("POST", "/api/v1/asks", `{"id":"tmdb:693134"}`, 201))
	req, _ := http.NewRequest("POST", r.srv.URL+"/api/v1/asks", strings.NewReader(`{"id":"tmdb:900002"}`))
	req.Header.Set(auth.CSRFHeader, r.ana.csrf)
	resp, err := r.ana.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 429 || resp.Header.Get("Retry-After") == "" || !strings.Contains(string(b), "which is your movie limit; try again in 7 days") ||
		!strings.Contains(string(b), `"kind":"movies"`) {
		t.Fatalf("over the limit: %d %s %s", resp.StatusCode, resp.Header.Get("Retry-After"), b)
	}
	var lim asks.Limit
	_ = json.Unmarshal(r.ana.call("GET", "/api/v1/asks/limit", "", 200), &lim)
	if m := lim.Movies; m.PerWeek != 1 || m.Left != 0 || m.RetryAt == nil || lim.Shows.Left != asks.DefaultPerWeek || lim.Music.Left != asks.DefaultPerWeek {
		t.Fatalf("limit %+v", lim)
	}

	// The older form gives ana 4 of each kind; then the owner gives her 3
	// movies a week and leaves shows and music on the default.
	r.owner.call("PUT", "/api/v1/ask-limits/"+itoa(r.ana.id), `{"per_week":4}`, 200)
	_ = json.Unmarshal(r.ana.call("GET", "/api/v1/asks/limit", "", 200), &lim)
	if lim.Movies.PerWeek != 4 || lim.Shows.PerWeek != 4 || lim.Music.PerWeek != 4 {
		t.Fatalf("older form %+v", lim)
	}
	r.owner.call("PUT", "/api/v1/ask-limits/"+itoa(r.ana.id), `{"movies_per_week":3,"shows_per_week":null,"music_per_week":null}`, 200)
	r.owner.call("PUT", "/api/v1/ask-limits/999", `{"movies_per_week":3}`, 404)
	r.owner.call("PUT", "/api/v1/ask-limits/"+itoa(r.ana.id), `{"shows_per_week":5000}`, 400)
	r.ana.call("PUT", "/api/v1/ask-limits/"+itoa(r.ana.id), `{"movies_per_week":0}`, 403)
	_ = json.Unmarshal(r.ana.call("GET", "/api/v1/asks/limit", "", 200), &lim)
	if lim.Movies.PerWeek != 3 || lim.Movies.Left != 2 || lim.Shows.PerWeek != asks.DefaultPerWeek || lim.Music.PerWeek != asks.DefaultPerWeek {
		t.Fatalf("own movie limit %+v", lim)
	}
	var limits struct {
		Limits []struct {
			Name    string         `json:"name"`
			Movies  *int           `json:"movies_per_week"`
			Shows   *int           `json:"shows_per_week"`
			Music   *int           `json:"music_per_week"`
			Used    map[string]int `json:"used"`
			NoLimit bool           `json:"no_limit"`
		} `json:"limits"`
	}
	_ = json.Unmarshal(r.owner.call("GET", "/api/v1/ask-limits", "", 200), &limits)
	seen := map[string]bool{}
	for _, l := range limits.Limits {
		switch l.Name {
		case "ana":
			seen["ana"] = l.Movies != nil && *l.Movies == 3 && l.Shows == nil && l.Music == nil && l.Used["movies"] == 1 && l.Used["shows"] == 0
		case "luis":
			seen["luis"] = l.NoLimit
		}
	}
	if !seen["ana"] || !seen["luis"] {
		t.Fatalf("limits %+v", limits)
	}
	r.ana.call("GET", "/api/v1/ask-limits", "", 403)
	r.ana.call("POST", "/api/v1/asks", `{"id":"tmdb:900002"}`, 201)

	// Turn down with a note; a member cannot take back someone else's ask.
	r.bo.call("DELETE", "/api/v1/asks/"+itoa(a.ID), "", 404)
	got := askOf(t, r.owner.call("POST", "/api/v1/asks/"+itoa(a.ID)+"/turn-down", `{"note":"we own the disc"}`, 200))
	if got.State != "turned_down" || got.Note != "we own the disc" {
		t.Fatalf("turned down %+v", got)
	}
	r.owner.call("POST", "/api/v1/asks/"+itoa(a.ID)+"/turn-down", `{}`, 409)
	r.ana.call("DELETE", "/api/v1/asks/"+itoa(a.ID), "", 409) // decided already

	c := askOf(t, r.ana.call("POST", "/api/v1/asks", `{"id":"tmdb:900003"}`, 201))
	r.ana.call("DELETE", "/api/v1/asks/"+itoa(c.ID), "", 204)
	r.ana.call("GET", "/api/v1/asks/"+itoa(c.ID), "", 404)
	r.asks.Wait()
}

// route: GET /api/v1/my/alerts
// route: PUT /api/v1/my/alerts
// route: GET /api/v1/alerts
// route: POST /api/v1/alerts
// route: PUT /api/v1/alerts/{id}
// route: DELETE /api/v1/alerts/{id}
// route: POST /api/v1/alerts/{id}/test
// route: PUT /api/v1/auth/settings
func TestAlertsPerMomentOverHTTP(t *testing.T) {
	r := newAskRig(t)
	var mu sync.Mutex
	var posts []string
	discord := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var m map[string]any
		_ = json.NewDecoder(req.Body).Decode(&m)
		mu.Lock()
		posts = append(posts, m["content"].(string))
		mu.Unlock()
		w.WriteHeader(204)
	}))
	defer discord.Close()
	got := func() []string { mu.Lock(); defer mu.Unlock(); return append([]string{}, posts...) }

	r.ana.call("PUT", "/api/v1/my/alerts", `{"email":"not an address"}`, 400)
	r.ana.call("PUT", "/api/v1/my/alerts", `{"discord_id":"abc"}`, 400)
	r.ana.call("PUT", "/api/v1/my/alerts", `{"email":"ana@example.test","discord_id":"112233445566"}`, 200)
	if b := r.ana.call("GET", "/api/v1/my/alerts", "", 200); !strings.Contains(string(b), "112233445566") {
		t.Fatalf("my alerts %s", b)
	}

	b := r.owner.call("POST", "/api/v1/alerts", `{"name":"Family","kind":"discord","allow_local_network":true,"url":"`+discord.URL+`/api/webhooks/9/secret-token"}`, 201)
	if strings.Contains(string(b), "secret-token") {
		t.Fatalf("the link leaked: %s", b)
	}
	var al AlertView
	_ = json.Unmarshal(b, &al)
	r.owner.call("POST", "/api/v1/alerts", `{"name":"family","kind":"discord","url":"https://discord.com/api/webhooks/1/x"}`, 409)
	// Off discord.com, or on the local network, a link needs the choice.
	r.owner.call("POST", "/api/v1/alerts", `{"name":"Other","kind":"discord","url":"https://x.test/hook"}`, 400)
	r.owner.call("POST", "/api/v1/alerts", `{"name":"Other","kind":"discord","url":"`+discord.URL+`/hook"}`, 400)
	r.owner.call("POST", "/api/v1/alerts", `{"name":"x","kind":"pager"}`, 400)
	r.ana.call("GET", "/api/v1/alerts", "", 403)
	if b := r.owner.call("GET", "/api/v1/alerts", "", 200); strings.Contains(string(b), "secret-token") || !strings.Contains(string(b), secrets.Masked) {
		t.Fatalf("list %s", b)
	}
	// Keeping the masked link keeps the stored one.
	r.owner.call("PUT", "/api/v1/alerts/"+itoa(al.ID), `{"name":"Family","url":"`+secrets.Masked+`"}`, 200)
	r.owner.call("PUT", "/api/v1/alerts/999", `{"name":"x"}`, 404)
	r.owner.call("POST", "/api/v1/alerts/"+itoa(al.ID)+"/test", "", 200)
	if p := got(); len(p) != 1 || !strings.Contains(p[0], "HolyArrs test") {
		t.Fatalf("test post %v", p)
	}

	// Each moment: an Event and a Discord message.
	a := askOf(t, r.ana.call("POST", "/api/v1/asks", `{"id":"tmdb:693134"}`, 201))
	r.asks.Wait()
	r.owner.call("POST", "/api/v1/asks/"+itoa(a.ID)+"/approve", `{}`, 200)
	r.asks.Wait()
	c := askOf(t, r.ana.call("POST", "/api/v1/asks", `{"id":"tmdb:900002"}`, 201))
	r.asks.Wait()
	r.owner.call("POST", "/api/v1/asks/"+itoa(c.ID)+"/turn-down", `{"note":"no"}`, 200)
	r.asks.Wait()
	var cur asks.View
	_ = json.Unmarshal(r.ana.call("GET", "/api/v1/asks/"+itoa(a.ID), "", 200), &cur)
	tid := *cur.TitleID
	err := r.st.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE slots SET state = 'in_library' WHERE item_id IN (SELECT id FROM items WHERE title_id = ?)", tid)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if n, err := r.asks.CheckReady(context.Background()); n != 1 || err != nil {
		t.Fatalf("ready %d %v", n, err)
	}
	r.asks.Wait()
	p := got()
	want := []string{"ana asked for Dune", "<@112233445566> **Approved: Dune", "ana asked for Paper Harbour", "<@112233445566> **Turned down: Paper Harbour", "<@112233445566> **Ready to watch: Dune"}
	if len(p) != 1+len(want) {
		t.Fatalf("posts %q", p)
	}
	for i, w := range want {
		if !strings.Contains(p[i+1], w) {
			t.Errorf("post %d %q, want %q", i+1, p[i+1], w)
		}
	}
	es, _ := r.st.RecentEvents(context.Background(), 50)
	kinds := map[string]bool{}
	for _, e := range es {
		kinds[e.Kind] = true
	}
	for _, k := range []string{domain.EventAskMade, domain.EventAskApproved, domain.EventAskTurnedDown, domain.EventAskAvailable} {
		if !kinds[k] {
			t.Errorf("no %s Event", k)
		}
	}
	var ms struct {
		Messages []asks.Message `json:"messages"`
	}
	_ = json.Unmarshal(r.ana.call("GET", "/api/v1/messages", "", 200), &ms)
	if len(ms.Messages) != 3 || ms.Messages[0].Kind != "ready" || ms.Messages[1].Kind != "turned_down" || ms.Messages[2].Kind != "approved" {
		t.Fatalf("messages %+v", ms.Messages)
	}

	// An email Alert with no address to test against says so.
	b = r.owner.call("POST", "/api/v1/alerts", `{"name":"Mail","kind":"email","smtp_host":"127.0.0.1","smtp_port":1,"smtp_security":"none","smtp_from":"h@example.test"}`, 201)
	var mail AlertView
	_ = json.Unmarshal(b, &mail)
	r.owner.call("POST", "/api/v1/alerts/"+itoa(mail.ID)+"/test", "", 400)
	r.owner.call("PUT", "/api/v1/alerts/"+itoa(mail.ID), `{"name":"Mail","smtp_host":"127.0.0.1","smtp_port":1,"smtp_security":"none","smtp_from":"h@example.test","staff_to":"luis@example.test"}`, 200)
	// A mail server on this machine without the local network choice is
	// refused; the answer is the same plain words every time.
	if b := r.owner.call("POST", "/api/v1/alerts/"+itoa(mail.ID)+"/test", "", 502); !strings.Contains(string(b), "the Discord, mail or webhook server refused the test") || strings.Contains(string(b), "127.0.0.1") {
		t.Fatalf("test answer %s", b)
	}
	r.owner.call("PUT", "/api/v1/alerts/"+itoa(mail.ID), `{"name":"Mail","allow_local_network":true,"smtp_host":"127.0.0.1","smtp_port":1,"smtp_security":"none","smtp_from":"h@example.test","staff_to":"luis@example.test"}`, 200)
	if b := r.owner.call("POST", "/api/v1/alerts/"+itoa(mail.ID)+"/test", "", 502); strings.Contains(string(b), "127.0.0.1") { // nothing listens on port 1
		t.Fatalf("test answer %s", b)
	}
	if b := r.owner.call("GET", "/api/v1/alerts", "", 200); strings.Contains(string(b), "127.0.0.1:1") || strings.Contains(string(b), "dial") {
		t.Fatalf("last error leaks detail: %s", b)
	}
	// Removing and testing need a password sign-in, not the local skip.
	r.owner.call("PUT", "/api/v1/auth/settings", `{"skip_login_local_network":true,"local_networks":["127.0.0.0/8","10.0.0.0/8"]}`, 200)
	jar, _ := cookiejar.New(nil)
	bare := &signedIn{t: t, base: r.srv.URL, c: &http.Client{Jar: jar}}
	if b := bare.call("DELETE", "/api/v1/alerts/"+itoa(mail.ID), "", 403); !strings.Contains(string(b), "sign in with your password") {
		t.Fatalf("weak delete %s", b)
	}
	if b := bare.call("POST", "/api/v1/alerts/"+itoa(mail.ID)+"/test", "", 403); !strings.Contains(string(b), "sign in with your password") {
		t.Fatalf("weak test %s", b)
	}
	r.owner.call("DELETE", "/api/v1/alerts/"+itoa(mail.ID), "", 204)
	r.owner.call("DELETE", "/api/v1/alerts/"+itoa(mail.ID), "", 404)
}

// A webhook Alert over HTTP: added with its link and secret (both masked
// afterwards), refused on the local network without the choice, and the
// test button posts a signed test message.
func TestWebhookAlertOverHTTP(t *testing.T) {
	r := newAskRig(t)
	var mu sync.Mutex
	var got []http.Header
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		mu.Lock()
		got = append(got, req.Header.Clone())
		mu.Unlock()
		w.WriteHeader(204)
	}))
	defer hook.Close()
	r.owner.call("POST", "/api/v1/alerts", `{"name":"Hook","kind":"webhook","url":"`+hook.URL+`/in"}`, 400)
	r.owner.call("POST", "/api/v1/alerts", `{"name":"Hook","kind":"webhook","webhook_auth":"basic","allow_local_network":true,"url":"`+hook.URL+`/in"}`, 400)
	b := r.owner.call("POST", "/api/v1/alerts", `{"name":"Hook","kind":"webhook","allow_local_network":true,"url":"`+hook.URL+`/in","secret":"hk-5678","moments":["grab","import","health"]}`, 201)
	if strings.Contains(string(b), "hk-5678") || strings.Contains(string(b), hook.URL) {
		t.Fatalf("a secret leaked: %s", b)
	}
	var al AlertView
	_ = json.Unmarshal(b, &al)
	if al.Secret != secrets.Masked || al.URL != secrets.Masked || al.WebhookAuth != "sign" || len(al.Moments) != 3 {
		t.Fatalf("view %+v", al)
	}
	r.owner.call("POST", "/api/v1/alerts/"+itoa(al.ID)+"/test", "", 200)
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || got[0].Get("X-HolyArrs-Event") != "test" || !strings.HasPrefix(got[0].Get("X-HolyArrs-Signature"), "sha256=") {
		t.Fatalf("test post %v", got)
	}
}
