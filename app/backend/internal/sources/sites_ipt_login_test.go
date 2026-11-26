package sources

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A Source that chose the browser cookie sends the pasted cookie as is, with
// the browser's user agent, and reads the search page.
func TestIPTorrentsCookieLogin(t *testing.T) {
	t.Parallel()
	srv := siteServer(t, "iptorrents.html", func(r *http.Request, _ string) {
		if r.URL.Path != "/t" || r.Header.Get("Cookie") != "uid=1234; pass=cookiepass; cf_clearance=abc" || r.Header.Get("User-Agent") != "Mozilla/5.0 Test" {
			t.Errorf("request %s cookie %q agent %q", r.URL, r.Header.Get("Cookie"), r.Header.Get("User-Agent"))
		}
	})
	c := New(Config{URL: srv.URL, Format: "site", Site: "iptorrents", Protocol: "torrent", AllowLocal: true,
		APIKey: "Cookie: uid=1234; pass=cookiepass; cf_clearance=abc;\n", SiteLogin: "cookie", SiteAgent: " Mozilla/5.0 Test "})
	items, err := c.Search(context.Background(), Query{Title: "Sample Show"})
	if err != nil || len(items) != 2 {
		t.Fatalf("items %v err %v", items, err)
	}
	if strings.Contains(items[0].Link, "cookiepass") {
		t.Fatalf("cookie in link %q", items[0].Link)
	}
	// A cookie with no "pass" part is still sent: the person chose the cookie.
	srv2 := siteServer(t, "iptorrents.html", func(r *http.Request, _ string) {
		if r.Header.Get("Cookie") != "theme=dark" {
			t.Errorf("cookie %q", r.Header.Get("Cookie"))
		}
	})
	plain := "theme" + "=dark"
	if _, err := New(Config{URL: srv2.URL, Format: "site", Site: "iptorrents", Protocol: "torrent", AllowLocal: true, APIKey: plain, SiteLogin: "cookie"}).
		Search(context.Background(), Query{Title: "x"}); err != nil {
		t.Fatal(err)
	}
}

// An expired cookie gets the sign-in page; the message says what to do.
func TestIPTorrentsCookieExpired(t *testing.T) {
	t.Parallel()
	srv := siteServer(t, "iptorrents_login.html", func(*http.Request, string) {})
	var se *Error
	_, err := New(Config{URL: srv.URL, Format: "site", Site: "iptorrents", Protocol: "torrent", AllowLocal: true, APIKey: "uid=1; pass=old", SiteLogin: "cookie"}).
		Search(context.Background(), Query{Title: "x"})
	if !errors.As(err, &se) || se.Kind != KindLoginRefused || se.Msg != iptCookieGone {
		t.Fatalf("err %v", err)
	}
	if strings.Contains(err.Error(), "old") {
		t.Fatalf("cookie in message %v", err)
	}
	// The site turning the call away outright says the same.
	deny := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) }))
	t.Cleanup(deny.Close)
	_, err = New(Config{URL: deny.URL, Format: "site", Site: "iptorrents", Protocol: "torrent", AllowLocal: true, APIKey: "uid=1; pass=old", SiteLogin: "cookie"}).
		Search(context.Background(), Query{Title: "x"})
	if !errors.As(err, &se) || se.Kind != KindLoginRefused {
		t.Fatalf("403 err %v", err)
	}
	// No cookie fails before any call.
	_, err = New(Config{URL: srv.URL, Format: "site", Site: "iptorrents", Protocol: "torrent", AllowLocal: true, SiteLogin: "cookie"}).
		Search(context.Background(), Query{Title: "x"})
	if !errors.As(err, &se) || se.Msg != "IPTorrents needs your browser cookie" {
		t.Fatalf("empty err %v", err)
	}
}

// A cookie Source gives its cookie and agent for fetching a Release file, even
// when the cookie holds something that looks like a passkey.
func TestFileHeaderIPTorrentsCookieLogin(t *testing.T) {
	ref := int64(1)
	ipt := domain.Source{URL: "https://iptorrents.com", Format: "site", SecretRef: &ref,
		Extra: map[string]string{"site": "iptorrents", extraSiteLogin: "cookie", extraSiteAgent: "Mozilla/5.0 Test", extraSiteUser: "77"}}
	h, err := FileHeader(context.Background(), ipt, oneSecret("uid=1; pass=p; tp=x"))
	if err != nil || h.Get("Cookie") != "uid=1; pass=p; tp=x" || h.Get("User-Agent") != "Mozilla/5.0 Test" {
		t.Fatalf("header %v, %v", h, err)
	}
	// The cookie's values are hidden from logs.
	cfg, _ := ConfigFor(context.Background(), ipt, oneSecret("uid=1234; pass=cookiepass"))
	if s := secretValues(cfg); !slices.Contains(s, "cookiepass") {
		t.Fatalf("secrets %v", s)
	}
}

// The add form's choice is stored, shown, kept on edits, and checked.
func TestSiteSourceLoginChoice(t *testing.T) {
	t.Parallel()
	srv, _ := sourceServer(t)
	raw := string(do(t, "POST", srv.URL+"/api/v1/sources", `{"name":"IPT C","format":"site","site":"iptorrents","site_login":"cookie","site_agent":"Mozilla/5.0 Test","api_key":"uid=1; pass=secretcookie"}`, 201))
	if strings.Contains(raw, "secretcookie") {
		t.Fatalf("cookie shown %s", raw)
	}
	var x SourceView
	if _ = json.Unmarshal([]byte(raw), &x); x.SiteLogin != "cookie" || x.SiteAgent != "Mozilla/5.0 Test" || x.APIKey == "" {
		t.Fatalf("added %+v", x)
	}
	id := strconv.FormatInt(x.ID, 10)
	_ = json.Unmarshal(do(t, "PUT", srv.URL+"/api/v1/sources/"+id, `{"name":"IPT C2"}`, 200), &x)
	if x.SiteLogin != "cookie" || x.SiteAgent != "Mozilla/5.0 Test" {
		t.Fatalf("after a rename %+v", x)
	}
	x = SourceView{}
	_ = json.Unmarshal(do(t, "PUT", srv.URL+"/api/v1/sources/"+id, `{"name":"IPT C2","site_login":"passkey","site_agent":"","site_user":"77","api_key":"newpass"}`, 200), &x)
	if x.SiteLogin != "" || x.SiteAgent != "" || x.SiteUser != "77" {
		t.Fatalf("after switching %+v", x)
	}
	do(t, "PUT", srv.URL+"/api/v1/sources/"+id, `{"name":"IPT C2","site_login":"password"}`, 400)
	do(t, "PUT", srv.URL+"/api/v1/sources/"+id, `{"name":"IPT C2","site_agent":"a\nb"}`, 400)
	do(t, "POST", srv.URL+"/api/v1/sources", `{"name":"K","format":"site","site":"knaben","site_login":"cookie"}`, 400)
	// A passkey Source saved without the field reads as user id + passkey.
	raw = string(do(t, "POST", srv.URL+"/api/v1/sources", `{"name":"IPT P","format":"site","site":"iptorrents","site_user":"5","api_key":"pk"}`, 201))
	if x = (SourceView{}); json.Unmarshal([]byte(raw), &x) != nil || x.SiteLogin != "" || x.SiteUser != "5" {
		t.Fatalf("passkey source %+v", x)
	}
}
