package alerts

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

type hookGot struct {
	header http.Header
	body   []byte
}

// fakeHook is a webhook server: it answers the codes in order (then 204)
// and keeps every request.
func fakeHook(t *testing.T, codes ...int) (*httptest.Server, func() []hookGot) {
	var mu sync.Mutex
	var got []hookGot
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, hookGot{r.Header.Clone(), b})
		n := len(got)
		mu.Unlock()
		if n <= len(codes) {
			w.WriteHeader(codes[n-1])
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	return srv, func() []hookGot { mu.Lock(); defer mu.Unlock(); return append([]hookGot{}, got...) }
}

func quick() []time.Duration {
	return []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
}

func movieWorld(t *testing.T, s *database.Store) int64 {
	t.Helper()
	var id int64
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		y := 2016
		ti := &mediafiles.Title{Kind: "movie", GateDriver: "none", Name: "Arrival", SortName: "arrival", Year: &y, MetadataSource: "local", NewPartsPolicy: "all",
			ReleaseGate: "released", WhyAdded: "by_hand", Status: "released", State: "active", SourceIDs: map[string]string{"tmdb": "329865"},
			Aliases: []string{}, MusicTypes: []string{}, Artwork: []string{}}
		var err error
		id, err = database.Insert(tx, "titles", ti, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// A webhook posts the moment, the movie and the Event's reasons and paths,
// signed with its secret.
func TestWebhookPayloadIsSigned(t *testing.T) {
	s, v := open(t)
	ctx := context.Background()
	srv, got := fakeHook(t)
	a, err := Add(ctx, s, v, Input{Name: "Hook", Kind: "webhook", URL: srv.URL + "/in", Secret: "hk-1234", AllowLocal: &yes}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if a.WebhookAuth != "sign" || a.PasswordSecretID == nil {
		t.Fatalf("alert %+v", a)
	}
	snd := &Sender{Store: s, Vault: v, Backoff: []time.Duration{}}
	tid := movieWorld(t, s)
	e := domain.Event{Kind: "import_done", TitleID: &tid, Summary: "Import done: 1 placed by hardlink",
		Data: map[string]any{"from": "/downloads/Arrival.2016.1080p", "paths": []any{map[string]any{"to": "/movies/Arrival (2016)/Arrival (2016).mkv"}}}}
	m, ok := snd.MessageFor(ctx, e)
	if !ok || m.Moment != MomentImport || m.Subject != "In the Library: Arrival (2016)" {
		t.Fatalf("message %+v", m)
	}
	if err := snd.Send(ctx, m); err != nil {
		t.Fatal(err)
	}
	g := got()
	if len(g) != 1 {
		t.Fatalf("posts %d", len(g))
	}
	var p Payload
	if err := json.Unmarshal(g[0].body, &p); err != nil {
		t.Fatal(err)
	}
	if p.Moment != "import" || p.Kind != "import_done" || p.Movie == nil || p.Movie.Name != "Arrival" || p.Movie.TMDB != "329865" ||
		!strings.Contains(string(g[0].body), `"to":"Arrival (2016).mkv"`) || strings.Contains(string(g[0].body), "/downloads") {
		t.Fatalf("payload %s", g[0].body)
	}
	ts, _ := strconv.ParseInt(g[0].header.Get(HeaderTimestamp), 10, 64)
	// 5.1.7 minor 8: the delivery id is signed too, so a captured message
	// cannot be sent again under a fresh id.
	mac := hmac.New(sha256.New, []byte("hk-1234"))
	mac.Write([]byte(g[0].header.Get(HeaderTimestamp) + "." + g[0].header.Get(HeaderDelivery) + "."))
	mac.Write(g[0].body)
	if len(g[0].header.Get(HeaderDelivery)) != 16 || g[0].header.Get(HeaderSignature) != "sha256="+hex.EncodeToString(mac.Sum(nil)) ||
		g[0].header.Get(HeaderSignature) != Sign("hk-1234", ts, g[0].header.Get(HeaderDelivery), g[0].body) || g[0].header.Get(HeaderEvent) != "import" {
		t.Fatalf("headers %v", g[0].header)
	}
	if strings.Contains(string(g[0].body), "hk-1234") || g[0].header.Get("Authorization") != "" {
		t.Fatal("the secret travelled in the message")
	}
	// Bearer sends the secret as the Authorization header instead.
	if _, err := Change(ctx, s, v, a.ID, Input{Name: "Hook", WebhookAuth: "bearer", Moments: a.Moments}, domain.System); err != nil {
		t.Fatal(err)
	}
	var b domain.Alert
	_ = database.Get(ctx, s.Read(), "alerts", a.ID, &b)
	if err := snd.SendOne(ctx, b, Message{Moment: MomentTest, Subject: "t", Text: "t"}); err != nil {
		t.Fatal(err)
	}
	if h := got()[1].header; h.Get("Authorization") != "Bearer hk-1234" || h.Get(HeaderSignature) != "" {
		t.Fatalf("bearer headers %v", h)
	}
}

// A failed send is tried again after each back-off wait while the server
// is down or busy; a refusal (4xx) is not.
func TestRetryWithBackOffPerKind(t *testing.T) {
	s, v := open(t)
	ctx := context.Background()
	for _, kind := range []string{"webhook", "discord"} {
		srv, got := fakeHook(t, 503, 429)
		in := Input{Name: "Retry " + kind, Kind: kind, URL: srv.URL + "/api/webhooks/1/abc", AllowLocal: &yes}
		a, err := Add(ctx, s, v, in, domain.System)
		if err != nil {
			t.Fatal(err)
		}
		snd := &Sender{Store: s, Vault: v, Backoff: quick()}
		if err := snd.SendOne(ctx, a, Message{Moment: MomentGrab, Subject: "x", Text: "y"}); err != nil || len(got()) != 3 {
			t.Fatalf("%s: err %v after %d tries", kind, err, len(got()))
		}
		// No new tries when there are none left.
		srv2, got2 := fakeHook(t, 500, 500, 500, 500, 500)
		in.Name, in.URL = "Down "+kind, srv2.URL+"/api/webhooks/1/abc"
		a2, _ := Add(ctx, s, v, in, domain.System)
		if err := snd.SendOne(ctx, a2, Message{Moment: MomentGrab, Subject: "x", Text: "y"}); err == nil || len(got2()) != 4 {
			t.Fatalf("%s down: err %v after %d tries", kind, err, len(got2()))
		}
		// A refusal is final.
		srv3, got3 := fakeHook(t, 400)
		in.Name, in.URL = "Refuses "+kind, srv3.URL+"/api/webhooks/1/abc"
		a3, _ := Add(ctx, s, v, in, domain.System)
		if err := snd.SendOne(ctx, a3, Message{Moment: MomentGrab, Subject: "x", Text: "y"}); err == nil || len(got3()) != 1 {
			t.Fatalf("%s 400: err %v after %d tries", kind, err, len(got3()))
		}
		// The test button tries once.
		srv4, got4 := fakeHook(t, 503)
		in.Name, in.URL = "Test "+kind, srv4.URL+"/api/webhooks/1/abc"
		a4, _ := Add(ctx, s, v, in, domain.System)
		if err := snd.TestOne(ctx, a4, Message{Moment: MomentTest, Subject: "x", Text: "y"}); err == nil || len(got4()) != 1 {
			t.Fatalf("%s test: err %v after %d tries", kind, err, len(got4()))
		}
	}
	// Email: a mail server that is busy for the first two connections.
	port, mails := fakeSMTP(t)
	busy := busySMTP(t, port, 2)
	a, err := Add(ctx, s, v, Input{Name: "Mail", Kind: "email", SMTPHost: "127.0.0.1", SMTPPort: busy, SMTPSecurity: "none", SMTPFrom: "h@a.test",
		StaffTo: "staff@a.test", AllowLocal: &yes}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	snd := &Sender{Store: s, Vault: v, Backoff: quick()}
	if err := snd.SendOne(ctx, a, Message{Moment: MomentImport, Subject: "In the Library: Arrival", Text: "placed"}); err != nil || len(mails()) != 1 {
		t.Fatalf("email: %v, %d mails", err, len(mails()))
	}
	if m := mails()[0]; len(m.to) != 1 || m.to[0] != "staff@a.test" || !strings.Contains(m.data, "In the Library: Arrival") {
		t.Fatalf("mail %+v", m)
	}
}

// busySMTP answers "421 busy" to the first n connections and passes the
// rest to the real fake mail server on port.
func busySMTP(t *testing.T, port, n int) int {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	var seen atomic.Int32
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			if int(seen.Add(1)) <= n {
				_, _ = io.WriteString(c, "421 busy, try later\r\n")
				c.Close()
				continue
			}
			go func(c net.Conn) {
				defer c.Close()
				up, err := net.Dial("tcp", "127.0.0.1:"+strconv.Itoa(port))
				if err != nil {
					return
				}
				defer up.Close()
				go func() { _, _ = io.Copy(up, c) }()
				_, _ = io.Copy(c, up)
			}(c)
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

// A webhook to this machine or a private address is refused unless the
// Alert may reach the local network, both when saved and when sent.
func TestWebhookPrivateLinkRefused(t *testing.T) {
	for _, u := range []string{"http://127.0.0.1:9/x", "http://localhost/x", "https://10.0.0.5/x", "http://192.168.1.2/x", "http://[::1]/x",
		"ftp://example.com/x", "https://user:pw@example.com/x", "not a link"} {
		if CheckWebhookURL(u, false) == nil {
			t.Errorf("%s accepted", u)
		}
	}
	if CheckWebhookURL("https://hooks.example.com/x", false) != nil || CheckWebhookURL("http://192.168.1.2/x", true) != nil {
		t.Error("good links refused")
	}
	s, v := open(t)
	ctx := context.Background()
	srv, got := fakeHook(t)
	if _, err := Add(ctx, s, v, Input{Name: "Local", Kind: "webhook", URL: srv.URL}, domain.System); err != ErrHook {
		t.Fatalf("local link saved: %v", err)
	}
	a, err := Add(ctx, s, v, Input{Name: "Local", Kind: "webhook", URL: srv.URL, AllowLocal: &yes}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	// Turned off later: the send refuses it too.
	a.AllowLocal = false
	snd := &Sender{Store: s, Vault: v, Backoff: quick()}
	if err := snd.SendOne(ctx, a, Message{Moment: MomentGrab, Subject: "x", Text: "y"}); err == nil || len(got()) != 0 {
		t.Fatalf("sent to a local server: %v %d", err, len(got()))
	}
}

// The secret and the link never reach the log, the Alert's last result or
// the alert_failed Event; a changed link host needs the secret again.
func TestWebhookSecretNeverLogged(t *testing.T) {
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	s, v := open(t)
	ctx := context.Background()
	srv, _ := fakeHook(t, 500)
	a, err := Add(ctx, s, v, Input{Name: "Hook", Kind: "webhook", URL: srv.URL + "/hook?token=lt-1234", Secret: "hk-1234", AllowLocal: &yes}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	snd := &Sender{Store: s, Vault: v, Backoff: []time.Duration{}}
	if err := snd.SendOne(ctx, a, Message{Moment: MomentGrab, Subject: "x", Text: "y"}); err == nil {
		t.Fatal("500 accepted")
	}
	// Down altogether: the client error holds the link.
	srv.Close()
	if err := snd.SendOne(ctx, a, Message{Moment: MomentGrab, Subject: "x", Text: "y"}); err == nil {
		t.Fatal("closed server accepted")
	}
	var b domain.Alert
	_ = database.Get(ctx, s.Read(), "alerts", a.ID, &b)
	es, _, _ := s.Events(ctx, database.EventFilter{Kinds: []string{domain.EventAlertFailed}})
	evs, _ := json.Marshal(es)
	for _, where := range []string{logs.String(), b.LastError, string(evs)} {
		if strings.Contains(where, "hk-1234") || strings.Contains(where, "lt-1234") {
			t.Fatalf("secret leaked: %s", where)
		}
	}
	if len(es) != 2 || b.LastError == "" {
		t.Fatalf("failures not recorded: %d events, last error %q", len(es), b.LastError)
	}
	// Another host keeps the secret only when it is typed again.
	if _, err := Change(ctx, s, v, a.ID, Input{Name: "Hook", URL: "https://elsewhere.example.com/hook", Moments: a.Moments}, domain.System); err != ErrSecretAgain {
		t.Fatalf("secret moved to another host: %v", err)
	}
	if _, err := Change(ctx, s, v, a.ID, Input{Name: "Hook", URL: "https://elsewhere.example.com/hook", Secret: "nw-1234", Moments: a.Moments}, domain.System); err != nil {
		t.Fatal(err)
	}
	// 5.1.7 minor 7: the same host over plain http needs the secret again.
	if _, err := Change(ctx, s, v, a.ID, Input{Name: "Hook", URL: "http://elsewhere.example.com/hook", Moments: a.Moments}, domain.System); err != ErrSecretAgain {
		t.Fatalf("secret kept on a downgrade to http: %v", err)
	}
}

// Stored Library Events reach the Alerts that speak for their moment:
// Discord, email (to the staff) and webhooks alike; others stay quiet.
func TestLibraryMomentsReachEveryKind(t *testing.T) {
	s, v := open(t)
	ctx := context.Background()
	hook, hooks := fakeHook(t)
	disc, posts := fakeDiscord(t)
	port, mails := fakeSMTP(t)
	only := func(ms ...string) []string { return ms }
	if _, err := Add(ctx, s, v, Input{Name: "Hook", Kind: "webhook", URL: hook.URL, AllowLocal: &yes,
		Moments: only(MomentGrab, MomentImport, MomentRename, MomentAdd, MomentDelete, MomentHealth, MomentDecision)}, domain.System); err != nil {
		t.Fatal(err)
	}
	if _, err := Add(ctx, s, v, Input{Name: "Discord", Kind: "discord", URL: disc.URL + "/api/webhooks/1/a", AllowLocal: &yes, Moments: only(MomentGrab, MomentHealth)}, domain.System); err != nil {
		t.Fatal(err)
	}
	if _, err := Add(ctx, s, v, Input{Name: "Mail", Kind: "email", SMTPHost: "127.0.0.1", SMTPPort: port, SMTPSecurity: "none", SMTPFrom: "h@a.test",
		StaffTo: "staff@a.test", AllowLocal: &yes, Moments: only(MomentImport)}, domain.System); err != nil {
		t.Fatal(err)
	}
	snd := &Sender{Store: s, Vault: v, Backoff: []time.Duration{}}
	snd.Watch(ctx)
	tid := movieWorld(t, s)
	for _, k := range []string{"download_sent", "import_done", domain.EventRenamed, domain.EventTitleAdded, domain.EventTitleRemoved,
		domain.EventHealthBroken, domain.EventHealthRecovered, domain.EventNeedsDecision, "search_done", "import_failed"} {
		if err := s.AddEvent(ctx, &domain.Event{Kind: k, TitleID: &tid, Summary: k}); err != nil {
			t.Fatal(err)
		}
	}
	snd.Wait()
	var moments []string
	for _, h := range hooks() {
		var p Payload
		_ = json.Unmarshal(h.body, &p)
		moments = append(moments, p.Moment)
	}
	if strings.Join(slices.Sorted(slices.Values(moments)), ",") != "add,delete,grab,health,health,import,needs_decision,rename" {
		t.Fatalf("webhook moments %v", moments)
	}
	if len(posts()) != 3 { // grab and two health
		t.Fatalf("discord posts %d", len(posts()))
	}
	if m := mails(); len(m) != 1 || m[0].to[0] != "staff@a.test" {
		t.Fatalf("mails %+v", m)
	}
}

// A webhook never carries a full path of this machine (the
// download folder, a Library folder, source files): only their last name.
func TestWebhookDataHasNoFullPaths(t *testing.T) {
	in := map[string]any{"from": "/downloads/complete/Lab Band - Old Days", "folder": `D:\Music\Lab Band`, "to": "Lab Band/Old Days (2010)/01 - a.flac",
		"paths": []any{"/downloads/x/01.flac", map[string]any{"source_file": "C:/dl/02.flac", "n": 3.0}}, "id": 5.0, "unc": `\nas\music\a.flac`}
	got, _ := json.Marshal(payloadData(in))
	want := `{"folder":"Lab Band","from":"Lab Band - Old Days","id":5,"paths":["01.flac",{"n":3,"source_file":"02.flac"}],"to":"Lab Band/Old Days (2010)/01 - a.flac","unc":"a.flac"}`
	if string(got) != want {
		t.Fatalf("%s", got)
	}
}
