package alerts

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

var yes = true

func open(t *testing.T) (*database.Store, *secrets.Vault) {
	t.Helper()
	// Breakers are process-wide and keyed by Alert id, which every fresh
	// database starts at 1 again: start and end each test with none.
	outbound.Reset()
	t.Cleanup(outbound.Reset)
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	v, err := secrets.Open(context.Background(), s, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	return s, v
}

// fakeDiscord records the JSON bodies posted to it.
func fakeDiscord(t *testing.T) (*httptest.Server, func() []map[string]any) {
	var mu sync.Mutex
	var got []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m map[string]any
		_ = json.NewDecoder(r.Body).Decode(&m)
		mu.Lock()
		got = append(got, m)
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	return srv, func() []map[string]any { mu.Lock(); defer mu.Unlock(); return append([]map[string]any{}, got...) }
}

type mailGot struct {
	from string
	to   []string
	data string
}

// fakeSMTP is a tiny mail server that keeps what it is sent.
func fakeSMTP(t *testing.T) (int, func() []mailGot) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	var mu sync.Mutex
	var got []mailGot
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				r := bufio.NewReader(c)
				say := func(s string) { _, _ = io.WriteString(c, s+"\r\n") }
				say("220 fake ready")
				var m mailGot
				for {
					line, err := r.ReadString('\n')
					if err != nil {
						return
					}
					cmd := strings.ToUpper(strings.TrimSpace(line))
					switch {
					case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
						say("250 fake")
					case strings.HasPrefix(cmd, "MAIL FROM:"):
						m.from = strings.Trim(strings.TrimSpace(line)[10:], "<>")
						say("250 ok")
					case strings.HasPrefix(cmd, "RCPT TO:"):
						m.to = append(m.to, strings.Trim(strings.TrimSpace(line)[8:], "<>"))
						say("250 ok")
					case cmd == "DATA":
						say("354 go on")
						var b strings.Builder
						for {
							l, err := r.ReadString('\n')
							if err != nil || l == ".\r\n" {
								break
							}
							b.WriteString(l)
						}
						m.data = b.String()
						mu.Lock()
						got = append(got, m)
						mu.Unlock()
						m = mailGot{}
						say("250 queued")
					case cmd == "QUIT":
						say("221 bye")
						return
					default:
						say("250 ok")
					}
				}
			}(c)
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port, func() []mailGot { mu.Lock(); defer mu.Unlock(); return append([]mailGot{}, got...) }
}

func TestDiscordNamesThePeopleWhoAsked(t *testing.T) {
	s, v := open(t)
	ctx := context.Background()
	srv, got := fakeDiscord(t)
	a, err := Add(ctx, s, v, Input{Name: "Family Discord", Kind: "discord", URL: srv.URL + "/api/webhooks/1/abc", AllowLocal: &yes}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Moments) != len(Moments) || a.URLSecretID == nil {
		t.Fatalf("alert %+v", a)
	}
	snd := &Sender{Store: s, Vault: v, Backoff: []time.Duration{}}
	err = snd.Send(ctx, Message{Moment: MomentReady, Subject: "Dune is ready", Text: "Watch it now.",
		People: []Person{{Name: "ana", DiscordID: "123456789012"}, {Name: "bo"}}})
	if err != nil {
		t.Fatal(err)
	}
	g := got()
	if len(g) != 1 {
		t.Fatalf("posts %v", g)
	}
	c, _ := g[0]["content"].(string)
	if !strings.HasPrefix(c, "<@123456789012> **Dune is ready**") || !strings.Contains(c, "Watch it now.") {
		t.Fatalf("content %q", c)
	}
	// The staff moment names nobody.
	_ = snd.Send(ctx, Message{Moment: MomentWaiting, Subject: "ana asked for Dune", Text: "x", People: []Person{{DiscordID: "123456789012"}}})
	if c, _ := got()[1]["content"].(string); strings.Contains(c, "<@") {
		t.Fatalf("staff content %q", c)
	}
	var fresh domain.Alert
	_ = database.Get(ctx, s.Read(), "alerts", a.ID, &fresh)
	if fresh.LastSentAt == nil || fresh.LastError != "" {
		t.Fatalf("result %+v", fresh)
	}
}

func TestMomentsOffAndFailuresAreRecorded(t *testing.T) {
	s, v := open(t)
	ctx := context.Background()
	srv, got := fakeDiscord(t)
	off := false
	if _, err := Add(ctx, s, v, Input{Name: "Off", Kind: "discord", URL: srv.URL, Enabled: &off, AllowLocal: &yes}, domain.System); err != nil {
		t.Fatal(err)
	}
	if _, err := Add(ctx, s, v, Input{Name: "Only ready", Kind: "discord", URL: srv.URL, Moments: []string{MomentReady}, AllowLocal: &yes}, domain.System); err != nil {
		t.Fatal(err)
	}
	snd := &Sender{Store: s, Vault: v, Backoff: []time.Duration{}}
	_ = snd.Send(ctx, Message{Moment: MomentApproved, Subject: "x", Text: "y"})
	if len(got()) != 0 {
		t.Fatalf("sent %v", got())
	}
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(500) }))
	defer dead.Close()
	bad, _ := Add(ctx, s, v, Input{Name: "Broken", Kind: "discord", URL: dead.URL + "/token-part", AllowLocal: &yes}, domain.System)
	err := snd.Send(ctx, Message{Moment: MomentApproved, Subject: "x", Text: "y"})
	if err == nil || !strings.Contains(err.Error(), "Discord answered 500") || strings.Contains(err.Error(), "token-part") {
		t.Fatalf("error %v", err)
	}
	var fresh domain.Alert
	_ = database.Get(ctx, s.Read(), "alerts", bad.ID, &fresh)
	if !strings.Contains(fresh.LastError, "500") {
		t.Fatalf("last error %q", fresh.LastError)
	}
	es, _ := s.RecentEvents(ctx, 10)
	if len(es) == 0 || es[0].Kind != domain.EventAlertFailed {
		t.Fatalf("events %+v", es)
	}
}

func TestEmailGoesToThePeopleOrTheStaff(t *testing.T) {
	s, v := open(t)
	ctx := context.Background()
	port, got := fakeSMTP(t)
	_, err := Add(ctx, s, v, Input{Name: "Mail", Kind: "email", SMTPHost: "127.0.0.1", SMTPPort: port, SMTPSecurity: "none",
		SMTPFrom: "holyarrs@example.test", StaffTo: "luis@example.test", AllowLocal: &yes}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	snd := &Sender{Store: s, Vault: v, Backoff: []time.Duration{}}
	err = snd.Send(ctx, Message{Moment: MomentApproved, Subject: "Approved: Dune", Text: "Line one\n.dot line",
		People: []Person{{Name: "ana", Email: "ana@example.test"}, {Name: "bo"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := snd.Send(ctx, Message{Moment: MomentWaiting, Subject: "ana asked for Dune", Text: "Approve it on the Requests page."}); err != nil {
		t.Fatal(err)
	}
	g := got()
	if len(g) != 2 {
		t.Fatalf("mails %+v", g)
	}
	if g[0].from != "holyarrs@example.test" || len(g[0].to) != 1 || g[0].to[0] != "ana@example.test" {
		t.Fatalf("first %+v", g[0])
	}
	if !strings.Contains(g[0].data, "Subject: Approved: Dune") || !strings.Contains(g[0].data, "\r\n..dot line\r\n") {
		t.Fatalf("data %q", g[0].data)
	}
	if g[1].to[0] != "luis@example.test" {
		t.Fatalf("staff mail %+v", g[1])
	}
	// Nobody with an address: nothing sent, no error.
	if err := snd.Send(ctx, Message{Moment: MomentReady, Subject: "x", Text: "y", People: []Person{{Name: "bo"}}}); err != nil || len(got()) != 2 {
		t.Fatalf("err %v, mails %d", err, len(got()))
	}
}

func TestAlertInputChecks(t *testing.T) {
	s, v := open(t)
	ctx := context.Background()
	for _, in := range []Input{
		{Name: "", Kind: "discord", URL: "https://x.test"},
		{Name: "a", Kind: "pager"},
		{Name: "a", Kind: "discord", URL: "ftp://x"},
		{Name: "a", Kind: "discord"},
		{Name: "a", Kind: "discord", URL: "https://x.test", Moments: []string{"nope"}},
		{Name: "a", Kind: "email", SMTPHost: "mail.test", SMTPFrom: "not an address"},
		{Name: "a", Kind: "email", SMTPHost: "mail.test", SMTPFrom: "a@b.test", SMTPSecurity: "ssl"},
		{Name: "a", Kind: "email", SMTPHost: "mail.test", SMTPFrom: "a@b.test", StaffTo: "x@y.test, bad"},
	} {
		if _, err := Add(ctx, s, v, in, domain.System); !IsInputError(err) {
			t.Errorf("%+v: %v", in, err)
		}
	}
	a, err := Add(ctx, s, v, Input{Name: "Mail", Kind: "email", SMTPHost: "mail.test", SMTPFrom: "a@b.test", SMTPUser: "u", Password: "pw-1"}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if a.SMTPPort != 587 || a.SMTPSecurity != "starttls" || a.PasswordSecretID == nil {
		t.Fatalf("defaults %+v", a)
	}
	if _, err := Add(ctx, s, v, Input{Name: "mail", Kind: "discord", URL: "https://discord.com/api/webhooks/1/x"}, domain.System); err != ErrNameUsed {
		t.Fatalf("same name: %v", err)
	}
	// An empty password keeps the stored one; a new one replaces it.
	b, err := Change(ctx, s, v, a.ID, Input{Name: "Mail", SMTPHost: "mail.test", SMTPFrom: "a@b.test", SMTPUser: "u"}, domain.System)
	if err != nil || *b.PasswordSecretID != *a.PasswordSecretID {
		t.Fatalf("change %+v %v", b, err)
	}
	b, _ = Change(ctx, s, v, a.ID, Input{Name: "Mail", SMTPHost: "mail.test", SMTPFrom: "a@b.test", Password: "pw-2"}, domain.System)
	if pw, _ := v.Get(ctx, *b.PasswordSecretID); pw != "pw-2" {
		t.Fatalf("password %q", pw)
	}
	if err := Remove(ctx, s, a.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = s.Read().QueryRow("SELECT count(*) FROM secrets WHERE kind LIKE 'alert_%'").Scan(&n)
	if n != 0 {
		t.Fatalf("%d secrets left", n)
	}
	if err := Remove(ctx, s, a.ID); err != database.ErrNotFound {
		t.Fatalf("again: %v", err)
	}
}

func TestDiscordLinksStayOnDiscord(t *testing.T) {
	for _, c := range []struct {
		url   string
		local bool
		ok    bool
	}{
		{"https://discord.com/api/webhooks/1/abc", false, true},
		{"https://discordapp.com/api/webhooks/1/abc", false, true},
		{"https://ptb.discord.com/api/webhooks/1/abc", false, true},
		{"http://discord.com/api/webhooks/1/abc", false, false},
		{"https://discord.com.evil.test/api/webhooks/1/abc", false, false},
		{"https://evildiscord.com/api/webhooks/1/abc", false, false},
		{"https://127.0.0.1/api/webhooks/1/abc", false, false},
		{"https://user@discord.com/x", false, false},
		{"http://127.0.0.1:8080/api/webhooks/1/abc", true, true},
		{"ftp://127.0.0.1/x", true, false},
	} {
		if err := CheckDiscordURL(c.url, c.local); (err == nil) != c.ok {
			t.Errorf("%s (local %v): %v", c.url, c.local, err)
		}
	}
}

func TestLocalServersNeedTheLocalNetworkChoice(t *testing.T) {
	s, v := open(t)
	ctx := context.Background()
	srv, _ := fakeDiscord(t)
	if _, err := Add(ctx, s, v, Input{Name: "D", Kind: "discord", URL: srv.URL + "/api/webhooks/1/abc"}, domain.System); !errors.Is(err, ErrURL) {
		t.Fatalf("local Discord link without the choice: %v", err)
	}
	// A mail server on this machine without the choice: never dialled.
	port, mails := fakeSMTP(t)
	a, err := Add(ctx, s, v, Input{Name: "Mail", Kind: "email", SMTPHost: "127.0.0.1", SMTPPort: port, SMTPSecurity: "none",
		SMTPFrom: "h@example.test", StaffTo: "luis@example.test"}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	snd := &Sender{Store: s, Vault: v, Backoff: []time.Duration{}}
	if err := snd.SendOne(ctx, a, Message{Moment: MomentWaiting, Subject: "x", Text: "y"}); err == nil || len(mails()) != 0 {
		t.Fatalf("sent to a local mail server: %v", err)
	}
	var fresh domain.Alert
	_ = database.Get(ctx, s.Read(), "alerts", a.ID, &fresh)
	if fresh.LastError == "" || strings.Contains(fresh.LastError, "127.0.0.1") {
		t.Fatalf("last error %q", fresh.LastError)
	}
}

func TestDiscordRedirectsAreNotFollowed(t *testing.T) {
	s, v := open(t)
	ctx := context.Background()
	target, posts := fakeDiscord(t)
	hop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/elsewhere", http.StatusTemporaryRedirect)
	}))
	defer hop.Close()
	a, err := Add(ctx, s, v, Input{Name: "Hop", Kind: "discord", URL: hop.URL + "/api/webhooks/1/secret-token", AllowLocal: &yes}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	snd := &Sender{Store: s, Vault: v, Backoff: []time.Duration{}}
	if err := snd.SendOne(ctx, a, Message{Moment: MomentWaiting, Subject: "x", Text: "y"}); err == nil || len(posts()) != 0 {
		t.Fatalf("redirect followed: %v, %d posts", err, len(posts()))
	}
	var fresh domain.Alert
	_ = database.Get(ctx, s.Read(), "alerts", a.ID, &fresh)
	es, _ := s.RecentEvents(ctx, 10)
	for _, txt := range []string{fresh.LastError, fmt.Sprint(es[0].Data)} {
		if strings.Contains(txt, "http") || strings.Contains(txt, "127.0.0.1") || strings.Contains(txt, "secret-token") {
			t.Fatalf("leaks a link: %q", txt)
		}
	}
}

func TestMailPasswordIsNotSentToANewServer(t *testing.T) {
	s, v := open(t)
	ctx := context.Background()
	base := Input{Name: "Mail", Kind: "email", SMTPHost: "mail.example.test", SMTPPort: 587, SMTPFrom: "a@b.test", SMTPUser: "u", Password: "pw-1"}
	a, err := Add(ctx, s, v, base, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	keep := base
	keep.Password = ""
	for name, change := range map[string]func(*Input){
		"host":     func(in *Input) { in.SMTPHost = "evil.example.test" },
		"port":     func(in *Input) { in.SMTPPort = 2525 },
		"user":     func(in *Input) { in.SMTPUser = "other" },
		"security": func(in *Input) { in.SMTPSecurity = "none" },
	} {
		in := keep
		change(&in)
		if _, err := Change(ctx, s, v, a.ID, in, domain.System); !errors.Is(err, ErrPasswordAgain) || !IsInputError(err) {
			t.Errorf("%s changed without the password: %v", name, err)
		}
	}
	// Same server: the stored password stays. A new server with the
	// password typed again is fine.
	if _, err := Change(ctx, s, v, a.ID, keep, domain.System); err != nil {
		t.Fatal(err)
	}
	in := keep
	in.SMTPHost = "new.example.test"
	in.Password = "pw-2"
	b, err := Change(ctx, s, v, a.ID, in, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if pw, _ := v.Get(ctx, *b.PasswordSecretID); pw != "pw-2" {
		t.Fatalf("password %q", pw)
	}
}
