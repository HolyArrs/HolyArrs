package accounts

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

func TestPluginActsOnlyForLinkedPeople(t *testing.T) {
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	a := New(s, Options{fastHash: true})
	t.Cleanup(a.Close)
	m := http.NewServeMux()
	m.HandleFunc("POST /api/v1/asks", func(w http.ResponseWriter, r *http.Request) {
		who, _ := webserver.From(r.Context())
		_ = json.NewEncoder(w).Encode(map[string]any{"user": who.UserID, "via": who.Via, "plugin": who.PluginID, "record_via": database.ViaPlugin(r.Context())})
	})
	m.HandleFunc("GET /api/v1/titles", okHandler)
	a.Routes(m)
	srv := httptest.NewServer(a.Middleware(webserver.RecordRouteHits(Gate(m))))
	t.Cleanup(srv.Close)
	c := client(t)
	csrf := setupOwner(t, &fixture{srv: srv, a: a, s: s}, c)

	tok := IssuePluginToken("bot", []string{"requests.change", "requests.users.read", "library.read"})
	t.Cleanup(func() { RevokePluginToken("bot") })
	asPlugin := map[string]string{"Authorization": "Bearer " + tok}
	with := func(h map[string]string, k, v string) map[string]string {
		out := map[string]string{k: v}
		for kk, vv := range h {
			out[kk] = vv
		}
		return out
	}

	resp, body := call(t, http.DefaultClient, "POST", srv.URL+"/api/v1/plugin-hooks/link", `{"external":"discord:42"}`, asPlugin)
	var got struct{ Code string }
	_ = json.Unmarshal([]byte(body), &got)
	if resp.StatusCode != 200 || len(got.Code) != 9 {
		t.Fatalf("link code: %d %s", resp.StatusCode, body)
	}
	// Before anyone links: refused.
	if resp, body := call(t, http.DefaultClient, "POST", srv.URL+"/api/v1/asks", `{}`, with(asPlugin, PluginForUserHeader, "1")); resp.StatusCode != 403 {
		t.Fatalf("unlinked: %d %s", resp.StatusCode, body)
	}
	// The plugin can't link itself or read links.
	if resp, _ := call(t, http.DefaultClient, "POST", srv.URL+"/api/v1/my/plugin-links", `{"code":"`+got.Code+`"}`, asPlugin); resp.StatusCode != 403 {
		t.Fatalf("plugin linked itself: %d", resp.StatusCode)
	}
	if resp, body := call(t, c, "POST", srv.URL+"/api/v1/my/plugin-links", `{"code":"`+got.Code+`"}`, map[string]string{CSRFHeader: csrf}); resp.StatusCode != 200 || !strings.Contains(body, `"plugin":"bot"`) {
		t.Fatalf("owner links: %d %s", resp.StatusCode, body)
	}
	if resp, _ := call(t, c, "POST", srv.URL+"/api/v1/my/plugin-links", `{"code":"`+got.Code+`"}`, map[string]string{CSRFHeader: csrf}); resp.StatusCode != 400 {
		t.Fatalf("a code worked twice: %d", resp.StatusCode)
	}
	for _, cse := range []struct {
		name, method, path, user string
		want                     int
	}{
		{"linked person, request", "POST", "/api/v1/asks", "1", 200},
		{"no person named", "POST", "/api/v1/asks", "", 403},
		{"someone who did not link", "POST", "/api/v1/asks", "99", 403},
		{"bad id", "POST", "/api/v1/asks", "x", 403},
		{"person on a non-person route", "GET", "/api/v1/titles", "1", 403},
		{"plain plugin read", "GET", "/api/v1/titles", "", 200},
	} {
		h := asPlugin
		if cse.user != "" {
			h = with(asPlugin, PluginForUserHeader, cse.user)
		}
		resp, body := call(t, http.DefaultClient, cse.method, srv.URL+cse.path, `{}`, h)
		if resp.StatusCode != cse.want {
			t.Errorf("%s: %d %s", cse.name, resp.StatusCode, body)
		}
		if cse.want == 200 && cse.path == "/api/v1/asks" && (!strings.Contains(body, `"plugin":"bot"`) || !strings.Contains(body, `"user":1`) || !strings.Contains(body, `"record_via":"bot"`)) {
			t.Errorf("%s: not recorded via the plugin: %s", cse.name, body)
		}
	}
	if resp, body := call(t, c, "GET", srv.URL+"/api/v1/my/plugin-links", "", nil); resp.StatusCode != 200 || !strings.Contains(body, `"external":"discord:42"`) {
		t.Errorf("my links: %d %s", resp.StatusCode, body)
	}
	// The plugin reads its own links, and only its own.
	links := func(tok string) (int, string) {
		resp, body := call(t, http.DefaultClient, "GET", srv.URL+"/api/v1/plugin-hooks/links", "", map[string]string{"Authorization": "Bearer " + tok})
		return resp.StatusCode, body
	}
	if code, body := links(tok); code != 200 || !strings.Contains(body, `"user_id":1`) || !strings.Contains(body, `"external":"discord:42"`) {
		t.Errorf("plugin links: %d %s", code, body)
	}
	other := IssuePluginToken("other", []string{"requests.users.read"})
	t.Cleanup(func() { RevokePluginToken("other") })
	if code, body := links(other); code != 200 || !strings.Contains(body, `"links":[]`) {
		t.Errorf("another plugin sees these links: %d %s", code, body)
	}
	noScope := IssuePluginToken("plain", []string{"library.read"})
	t.Cleanup(func() { RevokePluginToken("plain") })
	if code, _ := links(noScope); code != 403 {
		t.Errorf("links without requests.users.read: %d", code)
	}
	if resp, _ := call(t, c, "GET", srv.URL+"/api/v1/plugin-hooks/links", "", nil); resp.StatusCode != 403 {
		t.Errorf("a person read a plugin's links: %d", resp.StatusCode)
	}
	// Another plugin cannot remove the link; a bad id is refused.
	call(t, http.DefaultClient, "DELETE", srv.URL+"/api/v1/plugin-hooks/links/1", "", map[string]string{"Authorization": "Bearer " + other})
	if code, body := links(tok); !strings.Contains(body, `"user_id":1`) {
		t.Errorf("another plugin removed the link: %d %s", code, body)
	}
	if resp, _ := call(t, http.DefaultClient, "DELETE", srv.URL+"/api/v1/plugin-hooks/links/x", "", asPlugin); resp.StatusCode != 400 {
		t.Errorf("bad user id: %d", resp.StatusCode)
	}
	// The plugin removes its own link: it can no longer act for the person.
	if resp, body := call(t, http.DefaultClient, "DELETE", srv.URL+"/api/v1/plugin-hooks/links/1", "", asPlugin); resp.StatusCode != 200 || !strings.Contains(body, `"links":[]`) {
		t.Errorf("plugin unlink: %d %s", resp.StatusCode, body)
	}
	if resp, _ := call(t, http.DefaultClient, "POST", srv.URL+"/api/v1/asks", `{}`, with(asPlugin, PluginForUserHeader, "1")); resp.StatusCode != 403 {
		t.Errorf("after the plugin unlinked: %d", resp.StatusCode)
	}
	// Link again for the person's own unlink below.
	resp, body = call(t, http.DefaultClient, "POST", srv.URL+"/api/v1/plugin-hooks/link", `{"external":"discord:42"}`, asPlugin)
	_ = json.Unmarshal([]byte(body), &got)
	if resp, body := call(t, c, "POST", srv.URL+"/api/v1/my/plugin-links", `{"code":"`+got.Code+`"}`, map[string]string{CSRFHeader: csrf}); resp.StatusCode != 200 {
		t.Fatalf("owner links again: %d %s", resp.StatusCode, body)
	}
	// Unlinking ends it at once.
	call(t, c, "DELETE", srv.URL+"/api/v1/my/plugin-links/bot", "", map[string]string{CSRFHeader: csrf})
	if resp, _ := call(t, http.DefaultClient, "POST", srv.URL+"/api/v1/asks", `{}`, with(asPlugin, PluginForUserHeader, "1")); resp.StatusCode != 403 {
		t.Errorf("after unlink: %d", resp.StatusCode)
	}
	// Codes run out.
	code := newLinkCode("bot", "discord:7", time.Now().Add(-time.Hour))
	if _, ok := takeLinkCode(code, time.Now()); ok {
		t.Error("an old code worked")
	}
}

func TestLinkCodeGuessingCapped(t *testing.T) {
	now := time.Now()
	const who = 4242
	t.Cleanup(func() { linkMu.Lock(); delete(linkFails, who); linkMu.Unlock() })
	for i := range linkFailMax {
		if tooManyLinkFails(who, now) {
			t.Fatalf("capped after %d wrong codes", i)
		}
		linkFailed(who, now)
	}
	if !tooManyLinkFails(who, now) {
		t.Fatal("not capped after 10 wrong codes")
	}
	if tooManyLinkFails(who+1, now) {
		t.Error("another person is capped too")
	}
	if tooManyLinkFails(who, now.Add(linkLife+time.Second)) {
		t.Error("still capped after ten minutes")
	}
}
