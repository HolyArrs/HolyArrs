package alerts

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Made-up tokens in the shapes the apps give.
var (
	ntfyToken   = strings.Join([]string{"tk", "standinntfytokenfortests0123"}, "_")
	gotifyToken = strings.Join([]string{"AStandIn", "GotifyKey"}, ".")
	pushToken   = strings.Join([]string{"aStandInPushover", "AppToken0123ab"}, "")
	pushUser    = strings.Join([]string{"uStandInPushover", "UserKey0123abc"}, "")
)

type appCall struct {
	path   string
	header http.Header
	body   string
}

// fakeApp answers every request with answer (code and body) and keeps what
// it was sent.
func fakeApp(t *testing.T, answer func(n int) (int, string)) (string, func() []appCall) {
	var mu sync.Mutex
	var got []appCall
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, appCall{path: r.URL.Path, header: r.Header.Clone(), body: string(b)})
		n := len(got)
		mu.Unlock()
		code, body := answer(n)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL, func() []appCall { mu.Lock(); defer mu.Unlock(); return append([]appCall{}, got...) }
}

// headerBreak would start a new header if it reached one.
var headerBreak = strings.Join([]string{"a", "X-Evil: 1"}, "\r\n")

func ok200(int) (int, string) { return 200, `{}` }

// noSecret fails when any of the made-up secrets is in s.
func noSecret(t *testing.T, where, s string) {
	t.Helper()
	for _, x := range []string{ntfyToken, gotifyToken, pushToken, pushUser, "standinntfy", "StandInPushover"} {
		if strings.Contains(s, x) {
			t.Fatalf("a secret leaked into %s: %s", where, s)
		}
	}
}

// secretsKeptOut checks the error, its logged detail, the Alert's last
// result and the Event never hold a secret.
func secretsKeptOut(t *testing.T, s *database.Store, a domain.Alert, err error) {
	t.Helper()
	noSecret(t, "the error", err.Error())
	var f *failure
	if errors.As(err, &f) && f.detail != nil {
		noSecret(t, "the logged detail", f.detail.Error())
	}
	var fresh domain.Alert
	_ = database.Get(context.Background(), s.Read(), "alerts", a.ID, &fresh)
	noSecret(t, "the last result", fresh.LastError)
	evs, _ := database.Select[domain.Event](context.Background(), s.ReadEvents(), "events", "kind = ?", domain.EventAlertFailed)
	raw, _ := json.Marshal(evs)
	noSecret(t, "the event", string(raw))
}

func TestNtfySendsTextWithTitleAndClick(t *testing.T) {
	s, v := open(t)
	ctx := context.Background()
	base, got := fakeApp(t, ok200)
	a, err := Add(ctx, s, v, Input{Name: "Phone", Kind: "ntfy", ServerURL: base + "/", Topic: " holyarrs-alerts_1 ", AppToken: ntfyToken, AllowLocal: &yes}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if a.ServerURL != base || a.Topic != "holyarrs-alerts_1" || a.PasswordSecretID == nil {
		t.Fatalf("alert %+v", a)
	}
	snd := &Sender{Store: s, Vault: v, Backoff: []time.Duration{}}
	if err := snd.Send(ctx, Message{Moment: MomentReady, Subject: "Dune is ready", Text: "Watch it now.\nhttps://example.com/dune"}); err != nil {
		t.Fatal(err)
	}
	calls := got()
	if len(calls) != 1 || calls[0].path != "/holyarrs-alerts_1" {
		t.Fatalf("calls %+v", calls)
	}
	c := calls[0]
	if c.body != "Watch it now.\nhttps://example.com/dune" || c.header.Get("Title") != "Dune is ready" || c.header.Get("Click") != "https://example.com/dune" ||
		c.header.Get("Authorization") != "Bearer "+ntfyToken {
		t.Fatalf("sent %q %v", c.body, c.header)
	}
	// No token, no link: no Authorization and no Click; a title outside
	// ASCII goes as an RFC 2047 word.
	b, err := Add(ctx, s, v, Input{Name: "Open topic", Kind: "ntfy", ServerURL: base, Topic: "open", AllowLocal: &yes}, domain.System)
	if err != nil || b.PasswordSecretID != nil {
		t.Fatalf("add %+v %v", b, err)
	}
	if err := snd.SendOne(ctx, b, Message{Moment: MomentReady, Subject: "Amélie is ready", Text: "Watch it now."}); err != nil {
		t.Fatal(err)
	}
	c = got()[1]
	if c.header.Get("Authorization") != "" || c.header.Get("Click") != "" || !strings.HasPrefix(c.header.Get("Title"), "=?utf-8?b?") {
		t.Fatalf("sent %v", c.header)
	}
}

func TestNtfyDefaultsToNtfySh(t *testing.T) {
	s, v := open(t)
	a, err := Add(context.Background(), s, v, Input{Name: "Phone", Kind: "ntfy", Topic: "holyarrs"}, domain.System)
	if err != nil || a.ServerURL != "https://ntfy.sh" {
		t.Fatalf("alert %+v %v", a, err)
	}
}

func TestAppRefusals(t *testing.T) {
	cases := []struct {
		kind, name, body, want string
		code                   int
		retry                  bool
	}{
		{"ntfy", "refused token", `{"code":40101,"http":401,"error":"unauthorized ` + ntfyToken + `"}`, "ntfy refused the access token", 401, false},
		{"ntfy", "forbidden", `{"code":40301,"http":403,"error":"forbidden"}`, "ntfy refused the access token", 403, false},
		{"ntfy", "not found", `{"code":40401,"http":404,"error":"page not found"}`, "ntfy answered 404", 404, false},
		{"ntfy", "bad request", `{"error":"invalid request"}`, "ntfy answered 400", 400, false},
		{"ntfy", "slow down", `{"error":"limit reached"}`, "ntfy asked HolyArrs to slow down", 429, true},
		{"ntfy", "server error", `{}`, "ntfy answered 502", 502, true},
		{"gotify", "refused token", `{"error":"Unauthorized","errorCode":401,"errorDescription":"you need to provide a valid access token ` + gotifyToken + `"}`, "Gotify refused the app token", 401, false},
		{"gotify", "forbidden", `{"error":"Forbidden","errorCode":403}`, "Gotify refused the app token", 403, false},
		{"gotify", "bad message", `{"error":"Bad Request","errorCode":400}`, "Gotify refused the message", 400, false},
		{"gotify", "not found", `{}`, "Gotify answered 404", 404, false},
		{"gotify", "slow down", `{}`, "Gotify asked HolyArrs to slow down", 429, true},
		{"gotify", "server error", `{}`, "Gotify answered 500", 500, true},
		{"pushover", "refused token", `{"token":"invalid","errors":["application token is invalid: ` + pushToken + `"],"status":0}`, "Pushover refused the app token or user key", 400, false},
		{"pushover", "refused user", `{"user":"invalid","errors":["user identifier is not a valid user, group, or subscribed user key: ` + pushUser + `"],"status":0}`, "Pushover refused the app token or user key", 400, false},
		{"pushover", "other refusal", `{"errors":["message cannot be blank"],"status":0}`, "Pushover refused the message", 400, false},
		{"pushover", "slow down", `{}`, "Pushover asked HolyArrs to slow down", 429, true},
		{"pushover", "server error", `{}`, "Pushover answered 503", 503, true},
	}
	for _, c := range cases {
		t.Run(c.kind+" "+c.name, func(t *testing.T) {
			s, v := open(t)
			ctx := context.Background()
			base, got := fakeApp(t, func(int) (int, string) { return c.code, c.body })
			in := Input{Name: "A", Kind: c.kind, AllowLocal: &yes}
			switch c.kind {
			case "ntfy":
				in.ServerURL, in.Topic, in.AppToken = base, "holyarrs", ntfyToken
			case "gotify":
				in.ServerURL, in.AppToken = base, gotifyToken
			case "pushover":
				t.Setenv("HOLYARRS_PUSHOVER_URL", base)
				in.UserKey, in.AppToken = pushUser, pushToken
			}
			a, err := Add(ctx, s, v, in, domain.System)
			if err != nil {
				t.Fatal(err)
			}
			snd := &Sender{Store: s, Vault: v, Backoff: []time.Duration{}}
			err = snd.SendOne(ctx, a, Message{Moment: MomentWaiting, Subject: "Someone asks", Text: "Dune"})
			if err == nil || !strings.HasPrefix(err.Error(), c.want) || retryable(err) != c.retry {
				t.Fatalf("err %v (retry %v)", err, retryable(err))
			}
			if n := len(got()); n != 1 {
				t.Fatalf("tried %d times", n)
			}
			secretsKeptOut(t, s, a, err)
			// A final refusal is not tried again, even with waits to spare.
			if !c.retry {
				snd.Backoff = []time.Duration{time.Millisecond, time.Millisecond}
				_ = snd.SendOne(ctx, a, Message{Moment: MomentWaiting, Subject: "Someone asks", Text: "Dune"})
				if n := len(got()); n != 2 {
					t.Fatalf("tried %d times", n)
				}
			}
		})
	}
}

func TestAppServerErrorIsTriedAgain(t *testing.T) {
	for _, kind := range []string{"ntfy", "gotify", "pushover"} {
		t.Run(kind, func(t *testing.T) {
			s, v := open(t)
			ctx := context.Background()
			base, got := fakeApp(t, func(n int) (int, string) {
				if n == 1 {
					return 429, `{}`
				}
				if n == 2 {
					return 502, `{}`
				}
				return 200, `{"status":1}`
			})
			in := Input{Name: "A", Kind: kind, AllowLocal: &yes, ServerURL: base, Topic: "holyarrs", UserKey: pushUser}
			in.AppToken = map[string]string{"ntfy": ntfyToken, "gotify": gotifyToken, "pushover": pushToken}[kind]
			t.Setenv("HOLYARRS_PUSHOVER_URL", base)
			a, err := Add(ctx, s, v, in, domain.System)
			if err != nil {
				t.Fatal(err)
			}
			snd := &Sender{Store: s, Vault: v, Backoff: []time.Duration{time.Millisecond, time.Millisecond}}
			if err := snd.SendOne(ctx, a, Message{Moment: MomentWaiting, Subject: "Someone asks", Text: "Dune"}); err != nil {
				t.Fatal(err)
			}
			if n := len(got()); n != 3 {
				t.Fatalf("calls %d", n)
			}
		})
	}
}

func TestGotifySendsJSON(t *testing.T) {
	s, v := open(t)
	ctx := context.Background()
	base, got := fakeApp(t, ok200)
	a, err := Add(ctx, s, v, Input{Name: "Home", Kind: "gotify", ServerURL: base + "/gotify/", AppToken: gotifyToken, AllowLocal: &yes}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	snd := &Sender{Store: s, Vault: v, Backoff: []time.Duration{}}
	if err := snd.SendOne(ctx, a, Message{Moment: MomentReady, Subject: "Dune is ready", Text: "Watch it now."}); err != nil {
		t.Fatal(err)
	}
	c := got()[0]
	var body map[string]any
	_ = json.Unmarshal([]byte(c.body), &body)
	if c.path != "/gotify/message" || c.header.Get("X-Gotify-Key") != gotifyToken || body["title"] != "Dune is ready" || body["message"] != "Watch it now." || body["priority"] != float64(5) {
		t.Fatalf("sent %s %q %v", c.path, c.body, c.header)
	}
	if strings.Contains(c.body, gotifyToken) {
		t.Fatal("the token went into the body")
	}
}

func TestPushoverSendsForm(t *testing.T) {
	s, v := open(t)
	ctx := context.Background()
	base, got := fakeApp(t, func(int) (int, string) { return 200, `{"status":1,"request":"x"}` })
	t.Setenv("HOLYARRS_PUSHOVER_URL", base+"/")
	a, err := Add(ctx, s, v, Input{Name: "Push", Kind: "pushover", UserKey: " " + pushUser + " ", AppToken: pushToken, AllowLocal: &yes}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if a.UserKey != pushUser || a.PasswordSecretID == nil {
		t.Fatalf("alert %+v", a)
	}
	snd := &Sender{Store: s, Vault: v, Backoff: []time.Duration{}}
	if err := snd.SendOne(ctx, a, Message{Moment: MomentReady, Subject: "Dune is ready", Text: "Watch it now. https://example.com/dune"}); err != nil {
		t.Fatal(err)
	}
	c := got()[0]
	f, _ := url.ParseQuery(c.body)
	if c.path != "/1/messages.json" || f.Get("token") != pushToken || f.Get("user") != pushUser || f.Get("title") != "Dune is ready" ||
		f.Get("message") != "Watch it now. https://example.com/dune" || f.Get("url") != "https://example.com/dune" {
		t.Fatalf("sent %s %v", c.path, f)
	}
}

func TestAppNoAnswerKeepsSecretsOut(t *testing.T) {
	for _, kind := range []string{"ntfy", "gotify", "pushover"} {
		t.Run(kind, func(t *testing.T) {
			s, v := open(t)
			ctx := context.Background()
			t.Setenv("HOLYARRS_PUSHOVER_URL", "http://127.0.0.1:1")
			in := Input{Name: "A", Kind: kind, AllowLocal: &yes, ServerURL: "http://127.0.0.1:1", Topic: "holyarrs", UserKey: pushUser}
			in.AppToken = map[string]string{"ntfy": ntfyToken, "gotify": gotifyToken, "pushover": pushToken}[kind]
			a, err := Add(ctx, s, v, in, domain.System)
			if err != nil {
				t.Fatal(err)
			}
			snd := &Sender{Store: s, Vault: v, Backoff: []time.Duration{}}
			err = snd.SendOne(ctx, a, Message{Moment: MomentWaiting, Subject: "Someone asks", Text: "Dune"})
			if err == nil || !retryable(err) {
				t.Fatalf("err %v", err)
			}
			secretsKeptOut(t, s, a, err)
		})
	}
}

func TestAppLocalServerNeedsTheSwitch(t *testing.T) {
	s, v := open(t)
	ctx := context.Background()
	for _, in := range []Input{
		{Name: "A", Kind: "ntfy", ServerURL: "http://127.0.0.1:8080", Topic: "holyarrs"},
		{Name: "A", Kind: "gotify", ServerURL: "http://192.168.1.5", AppToken: gotifyToken},
		{Name: "A", Kind: "gotify", ServerURL: "http://gotify.local", AppToken: gotifyToken},
	} {
		if _, err := Add(ctx, s, v, in, domain.System); !errors.Is(err, ErrServer) {
			t.Fatalf("%+v: %v", in, err)
		}
	}
	// With the switch on it is saved; turned off, nothing is sent.
	base, got := fakeApp(t, ok200)
	a, err := Add(ctx, s, v, Input{Name: "A", Kind: "gotify", ServerURL: base, AppToken: gotifyToken, AllowLocal: &yes}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	a.AllowLocal = false
	snd := &Sender{Store: s, Vault: v, Backoff: []time.Duration{}}
	if err := snd.SendOne(ctx, a, Message{Moment: MomentWaiting, Subject: "Someone asks", Text: "Dune"}); !errors.Is(err, ErrServer) || len(got()) != 0 {
		t.Fatalf("err %v, calls %d", err, len(got()))
	}
}

func TestAppSaveChecks(t *testing.T) {
	s, v := open(t)
	ctx := context.Background()
	bad := []struct {
		in   Input
		want error
	}{
		{Input{Name: "A", Kind: "ntfy", Topic: ""}, ErrNtfyTopic},
		{Input{Name: "A", Kind: "ntfy", Topic: "a/b"}, ErrNtfyTopic},
		{Input{Name: "A", Kind: "ntfy", Topic: "../x"}, ErrNtfyTopic},
		{Input{Name: "A", Kind: "ntfy", Topic: strings.Repeat("a", 65)}, ErrNtfyTopic},
		{Input{Name: "A", Kind: "ntfy", Topic: "ok", AppToken: "has space"}, ErrNtfyToken},
		{Input{Name: "A", Kind: "ntfy", Topic: "ok", ServerURL: "ftp://ntfy.example.com"}, ErrServer},
		{Input{Name: "A", Kind: "ntfy", Topic: "ok", ServerURL: "https://ntfy.example.com/?auth=x"}, ErrServer},
		{Input{Name: "A", Kind: "ntfy", Topic: "ok", ServerURL: "https://me:pw@ntfy.example.com"}, ErrServer},
		{Input{Name: "A", Kind: "ntfy", Topic: "ok", ServerURL: "ntfy.example.com"}, ErrServer},
		{Input{Name: "A", Kind: "gotify", AppToken: gotifyToken}, ErrServer},
		{Input{Name: "A", Kind: "gotify", ServerURL: "https://gotify.example.com"}, ErrGotifyToken},
		{Input{Name: "A", Kind: "gotify", ServerURL: "https://gotify.example.com", AppToken: headerBreak}, ErrGotifyToken},
		{Input{Name: "A", Kind: "pushover", UserKey: pushUser}, ErrPushToken},
		{Input{Name: "A", Kind: "pushover", UserKey: pushUser, AppToken: "short"}, ErrPushToken},
		{Input{Name: "A", Kind: "pushover", AppToken: pushToken}, ErrPushUser},
		{Input{Name: "A", Kind: "pushover", AppToken: pushToken, UserKey: "not a key"}, ErrPushUser},
	}
	for _, c := range bad {
		if _, err := Add(ctx, s, v, c.in, domain.System); !errors.Is(err, c.want) || !IsInputError(err) {
			t.Fatalf("%+v: %v", c.in, err)
		}
	}
	// An empty token on change keeps the stored one.
	for _, in := range []Input{
		{Name: "N", Kind: "ntfy", ServerURL: "https://ntfy.example.com", Topic: "holyarrs", AppToken: ntfyToken},
		{Name: "G", Kind: "gotify", ServerURL: "https://gotify.example.com", AppToken: gotifyToken},
		{Name: "P", Kind: "pushover", UserKey: pushUser, AppToken: pushToken},
	} {
		a, err := Add(ctx, s, v, in, domain.System)
		if err != nil {
			t.Fatal(err)
		}
		was := *a.PasswordSecretID
		edit := in
		edit.Kind, edit.AppToken = "", ""
		if in.Kind == "ntfy" {
			edit.Topic = "other-topic"
		}
		b, err := Change(ctx, s, v, a.ID, edit, domain.System)
		if err != nil || b.PasswordSecretID == nil || *b.PasswordSecretID != was {
			t.Fatalf("change %+v %v", b, err)
		}
		if got, _ := v.Get(ctx, *b.PasswordSecretID); got != in.AppToken {
			t.Fatalf("%s: stored token changed", in.Kind)
		}
		if in.Kind == "ntfy" && b.Topic != "other-topic" {
			t.Fatalf("topic %q", b.Topic)
		}
		// The token never goes to another server without being typed again.
		if in.ServerURL != "" {
			edit.ServerURL = "https://elsewhere.example.com"
			if _, err := Change(ctx, s, v, a.ID, edit, domain.System); !errors.Is(err, ErrTokenAgain) || !IsInputError(err) {
				t.Fatalf("%s: moved server: %v", in.Kind, err)
			}
			edit.AppToken = in.AppToken
			if c, err := Change(ctx, s, v, a.ID, edit, domain.System); err != nil || c.ServerURL != "https://elsewhere.example.com" {
				t.Fatalf("%s: moved server with the token: %+v %v", in.Kind, c, err)
			}
		}
		if err := Remove(ctx, s, a.ID); err != nil {
			t.Fatal(err)
		}
	}
}

// A stored ntfy access token can be dropped (it is optional): the secret
// leaves the vault and the next send carries no Authorization. Gotify and
// Pushover tokens are needed, so asking to drop one without a new one is
// refused.
func TestNtfyTokenCanBeRemoved(t *testing.T) {
	s, v := open(t)
	ctx := context.Background()
	base, got := fakeApp(t, ok200)
	in := Input{Name: "Phone", Kind: "ntfy", ServerURL: base, Topic: "holyarrs", AppToken: ntfyToken, AllowLocal: &yes}
	a, err := Add(ctx, s, v, in, domain.System)
	if err != nil || a.PasswordSecretID == nil {
		t.Fatalf("add %+v %v", a, err)
	}
	was := *a.PasswordSecretID
	edit := in
	edit.Kind, edit.AppToken, edit.ClearToken = "", "", true
	b, err := Change(ctx, s, v, a.ID, edit, domain.System)
	if err != nil || b.PasswordSecretID != nil {
		t.Fatalf("change %+v %v", b, err)
	}
	var fresh domain.Alert
	if err := database.Get(ctx, s.Read(), "alerts", a.ID, &fresh); err != nil || fresh.PasswordSecretID != nil {
		t.Fatalf("stored %+v %v", fresh, err)
	}
	if _, err := v.Get(ctx, was); err == nil {
		t.Fatal("the dropped token is still in the vault")
	}
	snd := &Sender{Store: s, Vault: v, Backoff: []time.Duration{}}
	if err := snd.SendOne(ctx, fresh, Message{Moment: MomentReady, Subject: "Dune is ready", Text: "Watch it now."}); err != nil {
		t.Fatal(err)
	}
	if c := got(); len(c) != 1 || c[0].header.Get("Authorization") != "" {
		t.Fatalf("calls %+v", c)
	}
	// Dropping it while moving to another server needs no token.
	edit.ServerURL = "https://elsewhere.example.com"
	if _, err := Change(ctx, s, v, a.ID, Input{Name: "Phone", ServerURL: base, Topic: "holyarrs", AppToken: ntfyToken}, domain.System); err != nil {
		t.Fatal(err)
	}
	if c, err := Change(ctx, s, v, a.ID, edit, domain.System); err != nil || c.PasswordSecretID != nil || c.ServerURL != "https://elsewhere.example.com" {
		t.Fatalf("drop and move %+v %v", c, err)
	}
	// A new token sent with the box ticked replaces the stored one.
	edit.AppToken = ntfyToken
	if c, err := Change(ctx, s, v, a.ID, edit, domain.System); err != nil || c.PasswordSecretID == nil {
		t.Fatalf("new token %+v %v", c, err)
	}
	for _, in := range []Input{
		{Name: "G", Kind: "gotify", ServerURL: "https://gotify.example.com", AppToken: gotifyToken},
		{Name: "P", Kind: "pushover", UserKey: pushUser, AppToken: pushToken},
	} {
		x, err := Add(ctx, s, v, in, domain.System)
		if err != nil {
			t.Fatal(err)
		}
		e := in
		e.Kind, e.AppToken, e.ClearToken = "", "", true
		if _, err := Change(ctx, s, v, x.ID, e, domain.System); err == nil || !IsInputError(err) {
			t.Fatalf("%s: dropped a needed token: %v", in.Kind, err)
		}
		if y := mustGet(t, s, x.ID); y.PasswordSecretID == nil || *y.PasswordSecretID != *x.PasswordSecretID {
			t.Fatalf("%s: token changed %+v", in.Kind, y)
		}
	}
}

func mustGet(t *testing.T, s *database.Store, id int64) domain.Alert {
	t.Helper()
	var a domain.Alert
	if err := database.Get(context.Background(), s.Read(), "alerts", id, &a); err != nil {
		t.Fatal(err)
	}
	return a
}
