package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/alerts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// route: POST /api/v1/asks
// route: GET /api/v1/asks
// route: GET /api/v1/asks/{id}
// route: POST /api/v1/asks/{id}/approve
// route: GET /api/v1/messages
// route: POST /api/v1/messages/read
func TestAskJoinApproveOverHTTP(t *testing.T) {
	t.Parallel()
	r := newAskRig(t)
	a := askOf(t, r.ana.call("POST", "/api/v1/asks", `{"id":"tmdb:693134"}`, 201))
	if a.State != "asked" || a.AskedBy.Name != "ana" || !a.Mine {
		t.Fatalf("ask %+v", a)
	}
	r.ana.call("POST", "/api/v1/asks", `{"id":"imdb:tt1"}`, 400)
	r.ana.call("POST", "/api/v1/asks", `{"id":"tmdb:1"}`, 404)

	// bo joins; the lookup shows him the open ask.
	var look struct {
		Movies []discover.LookupView `json:"movies"`
	}
	_ = json.Unmarshal(r.bo.call("GET", "/api/v1/titles/lookup?term=tmdb:693134", "", 200), &look)
	if len(look.Movies) != 1 || look.Movies[0].Ask == nil || look.Movies[0].Ask.Mine || look.Movies[0].Ask.State != "asked" {
		t.Fatalf("lookup %+v", look)
	}
	// A member sees everyone who asked.
	b := r.bo.call("POST", "/api/v1/asks", `{"id":"tmdb:693134"}`, 200)
	if !strings.Contains(string(b), `"joined":true`) || len(askOf(t, b).People) != 2 {
		t.Fatalf("join %s", b)
	}

	// Members see only their own asks, and may not decide.
	r.bo.call("POST", "/api/v1/asks", `{"id":"tmdb:900003"}`, 201)
	var list struct {
		Asks []requests.View `json:"asks"`
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

	// The Movie page names who asked and who approved; a member sees
	// everyone who asked, and who decided by display name only.
	var tv TitleView
	_ = json.Unmarshal(r.ana.call("GET", "/api/v1/titles/"+itoa(*got.TitleID), "", 200), &tv)
	if len(tv.Asks) != 1 || len(tv.Asks[0].People) != 2 || tv.Asks[0].DecidedBy == nil || tv.Asks[0].DecidedBy.Name != "" || tv.WhyAdded != "ask" {
		t.Fatalf("title asks %+v", tv.Asks)
	}
	var owv TitleView
	_ = json.Unmarshal(r.owner.call("GET", "/api/v1/titles/"+itoa(*got.TitleID), "", 200), &owv)
	if len(owv.Asks) != 1 || owv.Asks[0].DecidedBy == nil || owv.Asks[0].DecidedBy.Name != "luis" {
		t.Fatalf("owner's title asks %+v", owv.Asks)
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
	t.Parallel()
	r := newAskRig(t)
	var set requests.Settings
	_ = json.Unmarshal(r.owner.call("GET", "/api/v1/settings/asks", "", 200), &set)
	if set.MoviesPerWeek != requests.DefaultPerWeek || set.ShowsPerWeek != requests.DefaultPerWeek || set.MusicPerWeek != requests.DefaultPerWeek || !set.SearchOnApprove {
		t.Fatalf("defaults %+v", set)
	}
	r.owner.call("PUT", "/api/v1/settings/asks", `{"movies_per_week":1,"search_on_approve":false}`, 200)
	r.owner.call("PUT", "/api/v1/settings/asks", `{"movies_per_week":-2}`, 400)
	r.owner.call("PUT", "/api/v1/settings/asks", `{"music_per_week":1001}`, 400)
	r.owner.call("PUT", "/api/v1/settings/asks", `{"movies_per_week":1,"library_folder_id":99}`, 409)
	r.ana.call("GET", "/api/v1/settings/asks", "", 403)

	a := askOf(t, r.ana.call("POST", "/api/v1/asks", `{"id":"tmdb:693134"}`, 201))
	req, _ := http.NewRequest("POST", r.srv.URL+"/api/v1/asks", strings.NewReader(`{"id":"tmdb:900002"}`))
	req.Header.Set(accounts.CSRFHeader, r.ana.csrf)
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
	var lim requests.Limit
	_ = json.Unmarshal(r.ana.call("GET", "/api/v1/asks/limit", "", 200), &lim)
	if m := lim.Movies; m.PerWeek != 1 || m.Left != 0 || m.RetryAt == nil || lim.Shows.Left != requests.DefaultPerWeek || lim.Music.Left != requests.DefaultPerWeek {
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
	if lim.Movies.PerWeek != 3 || lim.Movies.Left != 2 || lim.Shows.PerWeek != requests.DefaultPerWeek || lim.Music.PerWeek != requests.DefaultPerWeek {
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
	t.Parallel()
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
	var al alerts.View
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
	var cur requests.View
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
		Messages []requests.Message `json:"messages"`
	}
	_ = json.Unmarshal(r.ana.call("GET", "/api/v1/messages", "", 200), &ms)
	if len(ms.Messages) != 3 || ms.Messages[0].Kind != "ready" || ms.Messages[1].Kind != "turned_down" || ms.Messages[2].Kind != "approved" {
		t.Fatalf("messages %+v", ms.Messages)
	}

	// An email Alert with no address to test against says so.
	b = r.owner.call("POST", "/api/v1/alerts", `{"name":"Mail","kind":"email","smtp_host":"127.0.0.1","smtp_port":1,"smtp_security":"none","smtp_from":"h@example.test"}`, 201)
	var mail alerts.View
	_ = json.Unmarshal(b, &mail)
	r.owner.call("POST", "/api/v1/alerts/"+itoa(mail.ID)+"/test", "", 400)
	r.owner.call("PUT", "/api/v1/alerts/"+itoa(mail.ID), `{"name":"Mail","smtp_host":"127.0.0.1","smtp_port":1,"smtp_security":"none","smtp_from":"h@example.test","staff_to":"luis@example.test"}`, 200)
	// A mail server on this machine without the local network choice is
	// refused; the answer is the same plain words every time.
	if b := r.owner.call("POST", "/api/v1/alerts/"+itoa(mail.ID)+"/test", "", 502); !strings.Contains(string(b), "the Discord, Telegram, ntfy, Gotify, Pushover, mail or webhook server refused the test") || strings.Contains(string(b), "127.0.0.1") {
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
	bare.csrf = localCSRF(t, r.srv.URL, bare.c)
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
	t.Parallel()
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
	var al alerts.View
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

func TestTelegramAlertOverHTTP(t *testing.T) {
	r := newAskRig(t)
	token := strings.Join([]string{"123456789", "AAHstandInTokenForApiTests_01234"}, ":")
	var mu sync.Mutex
	var paths []string
	var texts []string
	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			ChatID string `json:"chat_id"`
			Text   string `json:"text"`
		}
		_ = json.NewDecoder(req.Body).Decode(&body)
		mu.Lock()
		paths, texts = append(paths, req.URL.Path), append(texts, body.ChatID+"|"+body.Text)
		mu.Unlock()
		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	defer tg.Close()
	t.Setenv("HOLYARRS_TELEGRAM_URL", tg.URL)
	r.owner.call("POST", "/api/v1/alerts", `{"name":"TG","kind":"telegram","chat_id":"42"}`, 400)
	body := func(m map[string]any) string { b, _ := json.Marshal(m); return string(b) }
	r.owner.call("POST", "/api/v1/alerts", body(map[string]any{"name": "TG", "kind": "telegram", "bot_token": token, "chat_id": "not a chat"}), 400)
	b := r.owner.call("POST", "/api/v1/alerts", body(map[string]any{"name": "TG", "kind": "telegram", "allow_local_network": true, "bot_token": token, "chat_id": "-1001234"}), 201)
	var al alerts.View
	_ = json.Unmarshal(b, &al)
	if al.BotToken != secrets.Masked || al.ChatID != "-1001234" || al.URL != "" {
		t.Fatalf("view %+v", al)
	}
	// The masked placeholder sent back keeps the stored token.
	r.owner.call("PUT", "/api/v1/alerts/"+itoa(al.ID), body(map[string]any{"name": "TG", "allow_local_network": true, "bot_token": secrets.Masked, "chat_id": "-1001234"}), 200)
	r.owner.call("POST", "/api/v1/alerts/"+itoa(al.ID)+"/test", "", 200)
	list := r.owner.call("GET", "/api/v1/alerts", "", 200)
	if strings.Contains(string(list)+string(b), "AAHstandIn") {
		t.Fatalf("the bot token leaked: %s", list)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(paths) != 1 || paths[0] != "/bot"+token+"/sendMessage" || !strings.HasPrefix(texts[0], "-1001234|HolyArrs test\n") {
		t.Fatalf("sent %v %v", paths, texts)
	}
}

// TestAppAlertsOverHTTP: ntfy, Gotify and Pushover Alerts are added, their
// tokens come back masked (never the real one), the masked value sent back
// keeps the stored token, and a test reaches each app.
func TestAppAlertsOverHTTP(t *testing.T) {
	r := newAskRig(t)
	ntfyTok := strings.Join([]string{"tk", "standinntfyapitoken0123"}, "_")
	gotifyTok := strings.Join([]string{"AStandIn", "GotifyApiKey"}, ".")
	pushTok := strings.Join([]string{"aStandInPushover", "ApiToken0123a"}, "")
	pushUser := strings.Join([]string{"uStandInPushover", "ApiUser01234a"}, "")
	var mu sync.Mutex
	var got []string
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		b, _ := io.ReadAll(req.Body)
		mu.Lock()
		got = append(got, req.URL.Path+"|"+req.Header.Get("Authorization")+req.Header.Get("X-Gotify-Key")+"|"+string(b))
		mu.Unlock()
		_, _ = w.Write([]byte(`{"status":1}`))
	}))
	defer app.Close()
	t.Setenv("HOLYARRS_PUSHOVER_URL", app.URL)
	body := func(m map[string]any) string { b, _ := json.Marshal(m); return string(b) }
	r.owner.call("POST", "/api/v1/alerts", `{"name":"N","kind":"ntfy","topic":"a/b"}`, 400)
	r.owner.call("POST", "/api/v1/alerts", body(map[string]any{"name": "G", "kind": "gotify", "server_url": app.URL, "app_token": gotifyTok}), 400) // local, switch off
	adds := []map[string]any{
		{"name": "N", "kind": "ntfy", "allow_local_network": true, "server_url": app.URL, "topic": "holyarrs", "app_token": ntfyTok},
		{"name": "G", "kind": "gotify", "allow_local_network": true, "server_url": app.URL, "app_token": gotifyTok},
		{"name": "P", "kind": "pushover", "allow_local_network": true, "user_key": pushUser, "app_token": pushTok},
	}
	var answers string
	var ntfyID int64
	for _, in := range adds {
		b := r.owner.call("POST", "/api/v1/alerts", body(in), 201)
		answers += string(b)
		var al alerts.View
		_ = json.Unmarshal(b, &al)
		if al.AppToken != secrets.Masked || al.URL != "" || al.BotToken != "" {
			t.Fatalf("view %+v", al)
		}
		switch al.Kind {
		case "ntfy":
			if al.ServerURL != app.URL || al.Topic != "holyarrs" {
				t.Fatalf("view %+v", al)
			}
			ntfyID = al.ID
		case "pushover":
			if al.UserKey != pushUser {
				t.Fatalf("view %+v", al)
			}
		}
		edit := map[string]any{}
		for k, v := range in {
			edit[k] = v
		}
		edit["app_token"] = secrets.Masked
		r.owner.call("PUT", "/api/v1/alerts/"+itoa(al.ID), body(edit), 200)
		r.owner.call("POST", "/api/v1/alerts/"+itoa(al.ID)+"/test", "", 200)
	}
	answers += string(r.owner.call("GET", "/api/v1/alerts", "", 200))
	for _, tok := range []string{ntfyTok, gotifyTok, pushTok} {
		if strings.Contains(answers, tok) {
			t.Fatalf("a token leaked: %s", answers)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 3 || got[0] != "/holyarrs|Bearer "+ntfyTok+"|This is a test from HolyArrs. If you can read it, the Alert N works." ||
		!strings.HasPrefix(got[1], "/message|"+gotifyTok+"|{") || !strings.HasPrefix(got[2], "/1/messages.json||") || pushForm(got[2]).Get("token") != pushTok {
		t.Fatalf("sent %q", got)
	}
	// The optional ntfy token can be dropped; the masked value sent with it
	// does not keep it.
	b := r.owner.call("PUT", "/api/v1/alerts/"+itoa(ntfyID), body(map[string]any{"name": "N", "allow_local_network": true, "server_url": app.URL, "topic": "holyarrs", "app_token": secrets.Masked, "clear_app_token": true}), 200)
	var al alerts.View
	_ = json.Unmarshal(b, &al)
	if al.AppToken != "" {
		t.Fatalf("token kept after removal: %+v", al)
	}
}

// pushForm reads the form a fake Pushover call was sent.
func pushForm(call string) url.Values {
	f, _ := url.ParseQuery(strings.TrimPrefix(call, "/1/messages.json||"))
	return f
}

// route: GET /api/v1/asks
// Who else declined the same title is told to people who approve
// only. A member's list and their own ask carry no also_declined at all.
func TestAlsoDeclinedForApproversOnly(t *testing.T) {
	t.Parallel()
	r := newAskRig(t)
	first := askOf(t, r.ana.call("POST", "/api/v1/asks", `{"id":"tmdb:693134"}`, 201))
	r.owner.call("POST", "/api/v1/asks/"+itoa(first.ID)+"/turn-down", `{}`, 200)
	again := askOf(t, r.bo.call("POST", "/api/v1/asks", `{"id":"tmdb:693134"}`, 201))
	if again.ID == first.ID {
		t.Fatalf("asked again is the same ask %d", again.ID)
	}
	list := func(s *signedIn) []map[string]json.RawMessage {
		var x struct {
			Asks []map[string]json.RawMessage `json:"asks"`
		}
		if err := json.Unmarshal(s.call("GET", "/api/v1/asks", "", 200), &x); err != nil {
			t.Fatal(err)
		}
		return x.Asks
	}
	for _, m := range []*signedIn{r.ana, r.bo} {
		for _, a := range list(m) {
			if _, ok := a["also_declined"]; ok {
				t.Fatalf("a member's list carries also_declined: %s", a["also_declined"])
			}
		}
	}
	if b := string(r.bo.call("GET", "/api/v1/asks/"+itoa(again.ID), "", 200)); strings.Contains(b, "also_declined") {
		t.Fatalf("a member's own ask carries also_declined: %s", b)
	}
	found := false
	for _, a := range list(r.owner) {
		// Every ask an approver reads carries the list, empty when nobody else declined.
		if _, ok := a["also_declined"]; !ok {
			t.Fatalf("an approver's ask has no also_declined: %v", a["id"])
		}
		var id int64
		_ = json.Unmarshal(a["id"], &id)
		if id != again.ID {
			continue
		}
		found = true
		var who []struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(a["also_declined"], &who); err != nil || len(who) != 1 || who[0].Name != "luis" {
			t.Fatalf("approver's also_declined %s %v", a["also_declined"], err)
		}
	}
	if !found {
		t.Fatal("the approver does not see the new ask")
	}
}

// route: GET /api/v1/asks
// The people who asked read who decided by display name only: their list
// carries no sign-in name and their message says "an admin" until the
// decider picks a display name.
func TestDeciderSignInNameHiddenFromMembers(t *testing.T) {
	t.Parallel()
	r := newAskRig(t)
	a := askOf(t, r.ana.call("POST", "/api/v1/asks", `{"id":"tmdb:693134"}`, 201))
	r.owner.call("POST", "/api/v1/asks/"+itoa(a.ID)+"/turn-down", `{}`, 200)
	decider := func(s *signedIn) (string, string) {
		var x struct {
			Asks []struct {
				ID        int64 `json:"id"`
				DecidedBy *struct {
					Name        string `json:"name"`
					DisplayName string `json:"display_name"`
				} `json:"decided_by"`
			} `json:"asks"`
		}
		if err := json.Unmarshal(s.call("GET", "/api/v1/asks", "", 200), &x); err != nil {
			t.Fatal(err)
		}
		for _, v := range x.Asks {
			if v.ID == a.ID && v.DecidedBy != nil {
				return v.DecidedBy.Name, v.DecidedBy.DisplayName
			}
		}
		t.Fatal("ask not listed")
		return "", ""
	}
	if n, _ := decider(r.ana); n != "" {
		t.Fatalf("a member reads the decider's sign-in name %q", n)
	}
	if n, _ := decider(r.owner); n != "luis" {
		t.Fatalf("an approver reads %q", n)
	}
	if m := string(r.ana.call("GET", "/api/v1/messages", "", 200)); !strings.Contains(m, "was turned down by an admin") || strings.Contains(m, "by luis") {
		t.Fatalf("message %s", m)
	}
	r.owner.call("PUT", "/api/v1/my/profile", `{"display_name":"Luis R"}`, 200)
	if n, d := decider(r.ana); n != "" || d != "Luis R" {
		t.Fatalf("with a display name the member reads %q %q", n, d)
	}
}

// route: GET /api/v1/asks
// An approved ask says where it is. The person who asked reads the
// step only; people who approve also read the search's words for why.
func TestAskStepOverHTTP(t *testing.T) {
	t.Parallel()
	r := newAskRig(t)
	a := askOf(t, r.ana.call("POST", "/api/v1/asks", `{"id":"tmdb:693134"}`, 201))
	r.owner.call("POST", "/api/v1/asks/"+itoa(a.ID)+"/approve", `{}`, 200)
	if err := r.st.Write(context.Background(), func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE jobs SET state = 'done'"); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE slots SET state = 'wanted', last_reason = 'no Release passed the Pick rules (1 found, 1 refused)'"); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE items SET last_searched_at = '2026-11-03T08:00:00.000000000Z'")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	step := func(s *signedIn) map[string]any {
		var x struct {
			Asks []struct {
				ID   int64          `json:"id"`
				Step map[string]any `json:"step"`
			} `json:"asks"`
		}
		if err := json.Unmarshal(s.call("GET", "/api/v1/asks", "", 200), &x); err != nil {
			t.Fatal(err)
		}
		for _, v := range x.Asks {
			if v.ID == a.ID {
				return v.Step
			}
		}
		t.Fatal("ask not listed")
		return nil
	}
	if s := step(r.ana); s["kind"] != "waiting" || s["text"] != nil {
		t.Fatalf("member's step %v", s)
	}
	if s := step(r.owner); s["kind"] != "waiting" || s["text"] != "no Release passed the Pick rules (1 found, 1 refused)" || s["searched_at"] == nil {
		t.Fatalf("approver's step %v", s)
	}
}
