package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProxyStripsHeadersAndMarks(t *testing.T) {
	var got http.Header
	var gotHost string
	back := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, gotHost = r.Header.Clone(), r.Host
	}))
	defer back.Close()
	u, _ := url.Parse(back.URL)
	who := func(*http.Request) (string, string) { return "amy@example.com", "amy-phone" }
	front := httptest.NewServer(newProxy(u, "secret-token-1234", who, func() string { return "holyarrs.tail1.ts.net" }))
	defer front.Close()

	req, _ := http.NewRequest("GET", front.URL+"/api/v1/titles", nil)
	for _, h := range []string{"X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "Forwarded", "X-Real-IP",
		"Remote-User", "Remote-Name", "X-Holyarrs-Outgoing", "X-Holyarrs-Tailscale-User", "X-Holyarrs-Tailscale-Addr", "Tailscale-User-Login",
		"Cf-Connecting-Ip", "True-Client-Ip"} {
		req.Header.Set(h, "10.0.0.5")
	}
	req.Header.Set(MarkHeader, "forged")
	req.Header.Set("Accept", "text/html")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	for _, h := range []string{"X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "Forwarded", "X-Real-IP",
		"Remote-User", "Remote-Name", "X-Holyarrs-Outgoing", "Tailscale-User-Login", "Cf-Connecting-Ip", "True-Client-Ip"} {
		if v := got.Values(h); len(v) > 0 {
			t.Errorf("%s reached HolyArrs: %q", h, v)
		}
	}
	if v := got.Values(MarkHeader); len(v) != 1 || v[0] != "secret-token-1234" {
		t.Errorf("mark = %q", v)
	}
	if got.Get(UserHeader) != "amy@example.com" || got.Get(DeviceHeader) != "amy-phone" {
		t.Errorf("identity = %q %q", got.Get(UserHeader), got.Get(DeviceHeader))
	}
	if got.Get("Accept") != "text/html" {
		t.Error("ordinary header dropped")
	}
	if gotHost != req.URL.Host {
		t.Errorf("host = %q, want the visitor's %q", gotHost, req.URL.Host)
	}
	if got.Get(AddrHeader) != "127.0.0.1" || got.Get(NameHeader) != "holyarrs.tail1.ts.net" {
		t.Errorf("addr/name = %q %q", got.Get(AddrHeader), got.Get(NameHeader))
	}
}

// A visitor cannot write the identity headers when WhoIs finds nobody.
func TestProxyNoForgedIdentity(t *testing.T) {
	var got http.Header
	back := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = r.Header.Clone() }))
	defer back.Close()
	u, _ := url.Parse(back.URL)
	front := httptest.NewServer(newProxy(u, "secret-token-1234", func(*http.Request) (string, string) { return "", "" }, nil))
	defer front.Close()
	req, _ := http.NewRequest("GET", front.URL, nil)
	req.Header.Set(UserHeader, "owner@example.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got.Get(UserHeader) != "" {
		t.Error("forged identity reached HolyArrs")
	}
}

func TestWriteStatus(t *testing.T) {
	dir := t.TempDir()
	exp := time.Date(2027, 1, 2, 3, 4, 5, 0, time.UTC)
	in := Status{State: StateConnected, DeviceName: "holyarrs", TSNetName: "holyarrs.tail1.ts.net",
		Version: "1.104.7", Updated: exp, LoginExpiry: exp, Connection: "direct"}
	if err := writeStatus(dir, in); err != nil {
		t.Fatal(err)
	}
	in.State = StateExpiring
	if err := writeStatus(dir, in); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "status.json"))
	if err != nil {
		t.Fatal(err)
	}
	var out Status
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Errorf("got %+v", out)
	}
	ents, _ := os.ReadDir(dir)
	if len(ents) != 1 {
		t.Errorf("leftover files: %d", len(ents))
	}
}

func TestPickState(t *testing.T) {
	n := time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		f    facts
		want string
	}{
		{facts{backend: "NoState"}, StateNotConnected},
		{facts{backend: "NeedsLogin", authURL: "https://login"}, StateNeedsSignIn},
		{facts{backend: "NeedsMachineAuth"}, StateNeedsAdmin},
		{facts{backend: "Running", keyExpiry: n.Add(100 * 24 * time.Hour)}, StateConnected},
		{facts{backend: "Running"}, StateConnected},
		{facts{backend: "Running", keyExpiry: n.Add(24 * time.Hour)}, StateExpiring},
		{facts{backend: "NeedsLogin", keyExpired: true}, StateExpired},
		{facts{backend: "NeedsLogin", hadNodeKey: true}, StateDeleted},
	} {
		if got := pickState(c.f, n); got != c.want {
			t.Errorf("%+v: %s, want %s", c.f, got, c.want)
		}
	}
}

func TestSignOutWipes(t *testing.T) {
	data := t.TempDir()
	st := stateDir(data)
	os.MkdirAll(filepath.Join(st, "certs"), 0o700)
	os.WriteFile(filepath.Join(st, "tailscaled.state"), []byte("key"), 0o600)
	called := false
	logoutErr, wipeErr := signOut(context.Background(), data, func(context.Context) error {
		called = true
		return errors.New("control unreachable")
	})
	if !called || logoutErr == nil || wipeErr != nil {
		t.Fatalf("called=%v logout=%v wipe=%v", called, logoutErr, wipeErr)
	}
	if _, err := os.Stat(st); !os.IsNotExist(err) {
		t.Error("state not wiped after failed logout")
	}
	b, _ := os.ReadFile(filepath.Join(data, "status.json"))
	var s Status
	json.Unmarshal(b, &s)
	if s.State != StateNotConnected {
		t.Errorf("status = %q", s.State)
	}
}

func TestSignOutCode(t *testing.T) {
	if c := signOutCode(nil); c != 0 {
		t.Errorf("logout worked: exit %d", c)
	}
	if c := signOutCode(errors.New("control unreachable")); c != 3 {
		t.Errorf("logout failed: exit %d, want 3", c)
	}
}

func TestRedact(t *testing.T) {
	if got := redact("visit https://login.tailscale.com/a/abc123 now tskey-auth-x"); got != "visit [hidden] now [hidden]" {
		t.Error(got)
	}
}

func TestRedactHeadscale(t *testing.T) {
	got := redact("control: https://hs.example.org:8080/register/nodekey:abc and (https://hs.example.org)")
	if strings.Contains(got, "https://") || strings.Contains(got, "hs.example.org") {
		t.Errorf("got %q", got)
	}
}

func TestReadAuthKeyDeletes(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, authKeyFile)
	if err := os.WriteFile(p, []byte("tskey-auth-xyz "), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOLYARRS_TAILSCALE_AUTHKEY", "from-env")
	if got := readAuthKey(dir); got != "tskey-auth-xyz" {
		t.Errorf("key = %q", got)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Error("auth key file not deleted")
	}
	if got := readAuthKey(dir); got != "" {
		t.Errorf("second read = %q (env must not be used)", got)
	}
}
