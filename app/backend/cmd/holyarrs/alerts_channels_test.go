package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/alerts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

type receiverCall struct {
	path   string
	header http.Header
	body   string
}

// alertReceivers answers as a webhook receiver (/hook, and /hook500 that always
// fails), the Telegram Bot API, an ntfy and a Gotify server and the
// Pushover API, and keeps every request.
func alertReceivers(t *testing.T) (*httptest.Server, func() []receiverCall) {
	var mu sync.Mutex
	var got []receiverCall
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, receiverCall{r.URL.Path, r.Header.Clone(), string(b)})
		mu.Unlock()
		switch {
		case r.URL.Path == "/hook500":
			w.WriteHeader(http.StatusInternalServerError)
		case r.URL.Path == "/hook":
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
		case r.URL.Path == "/1/messages.json":
			_, _ = w.Write([]byte(`{"status":1}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, func() []receiverCall { mu.Lock(); defer mu.Unlock(); return append([]receiverCall{}, got...) }
}

// TestAlertChannelsEndToEnd runs the whole app with no switch set. Each of
// webhook, Telegram, ntfy, Gotify and Pushover is added over the API and
// tested, then told about a finished download, an import and a failed
// download as the app records them. Every message reaches its stand-in
// server; the webhook's are signed and carry no full path of this machine.
func TestAlertChannelsEndToEnd(t *testing.T) {
	t.Setenv("HOLYARRS_FLAGS", "")
	srv, got := alertReceivers(t)
	t.Setenv("HOLYARRS_TELEGRAM_URL", srv.URL)
	t.Setenv("HOLYARRS_PUSHOVER_URL", srv.URL)
	var st *database.Store
	_, addr := startApp(t, config{}, runOpts{store: func(s *database.Store) { st = s }})
	jar, _ := cookiejar.New(nil)
	owner := &signedIn{t: t, base: "http://" + addr, c: &http.Client{Jar: jar, Timeout: 60 * time.Second}}
	var setup struct {
		CSRF string `json:"csrf"`
	}
	decodeJSON(t, owner.call("POST", "/api/v1/auth/setup", `{"name":"luis","password":"a long test passphrase"}`, 201), &setup)
	owner.csrf = setup.CSRF
	var me struct {
		Held []string `json:"held"`
	}
	decodeJSON(t, owner.call("GET", "/api/v1/auth/me", "", 200), &me)
	if slices.Contains(me.Held, "alert_channels_more") {
		t.Fatalf("alert channels are held back: %v", me.Held)
	}

	signWith := strings.Join([]string{"e2e", "hook", "sign"}, "-")
	tgToken := strings.Join([]string{"123456789", "AAHstandInTokenForEndToEnd_0123"}, ":")
	gotifyTok := strings.Join([]string{"AStandIn", "GotifyEndToEnd"}, ".")
	pushTok := strings.Join([]string{"aStandInPushover", "EndToEnd012ab"}, "")
	pushUser := strings.Join([]string{"uStandInPushover", "EndToEnd012cd"}, "")
	moments := []string{alerts.MomentDownloaded, alerts.MomentImport, alerts.MomentDownloadFailed}
	adds := []map[string]any{
		{"name": "Hook", "kind": "webhook", "url": srv.URL + "/hook", "secret": signWith},
		{"name": "TG", "kind": "telegram", "bot_token": tgToken, "chat_id": "-1001234"},
		{"name": "N", "kind": "ntfy", "server_url": srv.URL, "topic": "holyarrs"},
		{"name": "G", "kind": "gotify", "server_url": srv.URL, "app_token": gotifyTok},
		{"name": "P", "kind": "pushover", "user_key": pushUser, "app_token": pushTok},
	}
	for _, in := range adds {
		in["allow_local_network"], in["moments"] = true, moments
		b, _ := json.Marshal(in)
		var al alerts.View
		decodeJSON(t, owner.call("POST", "/api/v1/alerts", string(b), 201), &al)
		owner.call("POST", "/api/v1/alerts/"+strconv.FormatInt(al.ID, 10)+"/test", "", 200)
	}
	// A receiver that answers 500: the test button says so and the card
	// keeps the reason.
	var down alerts.View
	decodeJSON(t, owner.call("POST", "/api/v1/alerts", `{"name":"Hook down","kind":"webhook","allow_local_network":true,"url":"`+srv.URL+`/hook500","moments":[]}`, 201), &down)
	owner.call("POST", "/api/v1/alerts/"+strconv.FormatInt(down.ID, 10)+"/test", "", 502)
	var list struct {
		Alerts []alerts.View `json:"alerts"`
	}
	decodeJSON(t, owner.call("GET", "/api/v1/alerts", "", 200), &list)
	for _, a := range list.Alerts {
		want := ""
		if a.ID == down.ID {
			want = "the webhook server answered 500"
		}
		if a.LastError != want || (want == "") != (a.LastSentAt != nil) {
			t.Errorf("%s: last error %q, last sent %v", a.Name, a.LastError, a.LastSentAt)
		}
	}

	// The Events as the download manager and the importer record them.
	tid := addTitleFor(t, st, "Arrival")
	ctx := context.Background()
	src := "/downloads/complete/Arrival.2016.1080p/Arrival.2016.1080p.mkv"
	for _, e := range []*domain.Event{
		{Kind: "download_finished", TitleID: &tid, Summary: "Download abcd1234: finished", Data: map[string]any{"state": "finished", "progress": 1.0,
			"save_path": "/downloads/complete/Arrival.2016.1080p", "release": "Arrival.2016.1080p.WEB", "release_id": int64(7), "downloader": "qBit"}},
		{Kind: "import_done", TitleID: &tid, Summary: "Import done: 1 placed by hardlink", Data: map[string]any{"import_id": int64(3),
			"from":   "/downloads/complete/Arrival.2016.1080p",
			"paths":  []map[string]string{{"from": src, "to": "/movies/Arrival/Arrival.mkv", "method": "hardlink"}},
			"result": []mediafiles.PlaceResult{{SourceFile: src, Outcome: "placed", PlacedBy: "hardlink"}}}},
		{Kind: "download_failed", TitleID: &tid, Summary: "Download failed: no progress for 6 hours", Data: map[string]any{"state": "failed",
			"reason": "no progress for 6 hours", "release": "Arrival.2016.1080p.WEB", "downloader": "qBit"}},
	} {
		if err := st.AddEvent(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	channel := func(c receiverCall) string {
		switch {
		case strings.HasPrefix(c.path, "/hook500"):
			return "hook down"
		case strings.HasPrefix(c.path, "/hook"):
			return "webhook"
		case strings.HasSuffix(c.path, "/sendMessage"):
			return "telegram"
		case c.path == "/holyarrs":
			return "ntfy"
		case c.path == "/message":
			return "gotify"
		case c.path == "/1/messages.json":
			return "pushover"
		}
		return c.path
	}
	byChannel := func() map[string][]receiverCall {
		out := map[string][]receiverCall{}
		for _, c := range got() {
			out[channel(c)] = append(out[channel(c)], c)
		}
		return out
	}
	kinds := []string{"webhook", "telegram", "ntfy", "gotify", "pushover"}
	deadline := time.Now().Add(30 * time.Second)
	for {
		cs, all := byChannel(), true
		for _, k := range kinds {
			all = all && len(cs[k]) >= 4 // the test, then the three Events
		}
		if all {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("not every channel got its 4 messages: %v", cs)
		}
		time.Sleep(50 * time.Millisecond)
	}
	cs := byChannel()
	if len(cs["hook down"]) != 1 {
		t.Fatalf("the failing receiver got %d posts, want only the test", len(cs["hook down"]))
	}
	for _, k := range kinds[1:] {
		var text string
		for _, c := range cs[k] {
			body := c.body
			if k == "pushover" {
				body, _ = url.QueryUnescape(body) // a form
			}
			text += c.header.Get("Title") + " " + body + "\n"
		}
		if len(cs[k]) != 4 {
			t.Errorf("%s: %d messages, want 4", k, len(cs[k]))
		}
		for _, want := range []string{"HolyArrs test", "Downloaded: Arrival", "In the Library: Arrival", "Download failed: Arrival"} {
			if !strings.Contains(text, want) {
				t.Errorf("%s: no %q in %s", k, want, text)
			}
		}
	}
	events := map[string]bool{}
	for _, c := range cs["webhook"] {
		h := c.header
		ts, _ := strconv.ParseInt(h.Get(alerts.HeaderTimestamp), 10, 64)
		if h.Get(alerts.HeaderSignature) != alerts.Sign(signWith, ts, h.Get(alerts.HeaderDelivery), []byte(c.body)) {
			t.Errorf("%s: bad signature", h.Get(alerts.HeaderEvent))
		}
		var p alerts.Payload
		if err := json.Unmarshal([]byte(c.body), &p); err != nil {
			t.Fatal(err)
		}
		events[p.Moment] = true
		if p.Moment != h.Get(alerts.HeaderEvent) || strings.Contains(c.body, "/downloads") || strings.Contains(c.body, "/movies") || strings.Contains(c.body, signWith) {
			t.Errorf("payload %s", c.body)
		}
		if p.Moment != "test" && (p.Movie == nil || p.Movie.Name != "Arrival" || p.Kind == "") {
			t.Errorf("payload %s", c.body)
		}
	}
	for _, m := range []string{"test", alerts.MomentDownloaded, alerts.MomentImport, alerts.MomentDownloadFailed} {
		if !events[m] {
			t.Errorf("the webhook got no %s message: %v", m, events)
		}
	}
}
