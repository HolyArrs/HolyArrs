package health

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

func proxyServer(t *testing.T, role string) (*httptest.Server, *database.Store, *secrets.Vault) {
	t.Helper()
	dir := t.TempDir()
	s, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	key, _ := secrets.LoadKey(dir, "")
	v, err := secrets.Open(context.Background(), s, key)
	if err != nil {
		t.Fatal(err)
	}
	m := http.NewServeMux()
	(&ProxyHandlers{Store: s, Secrets: v}).Routes(m)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.ServeHTTP(w, r.WithContext(webserver.ContextWith(r.Context(), webserver.Caller{UserID: 1, Role: role})))
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { outbound.SetProxy(nil) })
	return srv, s, v
}

func proxyCall(t *testing.T, srv *httptest.Server, method, body string, want int) string {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+"/api/v1/settings/proxy", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != want {
		t.Fatalf("%s: %d, want %d: %s", method, resp.StatusCode, want, b)
	}
	return string(b)
}

// route: GET /api/v1/settings/proxy
// route: PUT /api/v1/settings/proxy
func TestProxySettingsDefaultOff(t *testing.T) {
	srv, _, _ := proxyServer(t, webserver.RoleOwner)
	var got ProxyAnswer
	_ = json.Unmarshal([]byte(proxyCall(t, srv, "GET", "", 200)), &got)
	if got.Type != "" || !got.BypassLocal || got.Password != "" {
		t.Fatalf("default = %+v, want off with local bypass on", got)
	}
	if outbound.CurrentProxy() != nil {
		t.Fatal("proxy on by default")
	}
}

// testPass is a made-up proxy sign-in; mask is the stand-in the API shows.
const testPass, mask = "pa55-word", secrets.Masked

func TestProxySettingsSaveAppliesAndHidesPassword(t *testing.T) {
	srv, s, v := proxyServer(t, webserver.RoleOwner)
	body := proxyCall(t, srv, "PUT", fmt.Sprintf(`{"type":"socks5","host":"proxy.lan","port":1080,"username":"me","password":%q,"bypass":"*.home.test","bypass_local":true}`, testPass), 200)
	if strings.Contains(body, testPass) {
		t.Fatalf("answer shows the password: %s", body)
	}
	p := outbound.CurrentProxy()
	if p == nil || p.String() != "socks5://proxy.lan:1080" {
		t.Fatalf("applied %v", p)
	}
	if got := proxyCall(t, srv, "GET", "", 200); strings.Contains(got, testPass) || !strings.Contains(got, fmt.Sprintf(`"password":%q`, mask)) {
		t.Fatalf("GET = %s", got)
	}
	// The password sits in the vault, never in the settings row.
	raw, _ := database.Setting(context.Background(), s.Read(), "this_app", "proxy")
	if strings.Contains(raw, testPass) {
		t.Fatalf("settings row holds the password: %s", raw)
	}
	if pw, _ := v.Setting(context.Background(), "this_app", "proxy_password"); pw != testPass {
		t.Fatalf("vault = %q", pw)
	}
	// Sending the mask back keeps the password; a new start applies it again.
	proxyCall(t, srv, "PUT", fmt.Sprintf(`{"type":"socks5","host":"proxy.lan","port":1081,"username":"me","password":%q,"bypass_local":true}`, mask), 200)
	if pw, _ := v.Setting(context.Background(), "this_app", "proxy_password"); pw != testPass {
		t.Fatalf("mask overwrote the password: %q", pw)
	}
	outbound.SetProxy(nil)
	if err := LoadProxy(context.Background(), s, v); err != nil || outbound.CurrentProxy().String() != "socks5://proxy.lan:1081" {
		t.Fatalf("LoadProxy: %v %v", outbound.CurrentProxy(), err)
	}
	// Off turns it off and clears the password.
	proxyCall(t, srv, "PUT", `{"type":"","password":""}`, 200)
	if outbound.CurrentProxy() != nil {
		t.Fatal("still on")
	}
	if pw, _ := v.Setting(context.Background(), "this_app", "proxy_password"); pw != "" {
		t.Fatalf("password kept: %q", pw)
	}
}

func TestProxySettingsRefusesBadInput(t *testing.T) {
	srv, _, _ := proxyServer(t, webserver.RoleOwner)
	for _, b := range []string{`{"type":"ftp","host":"h","port":1}`, `{"type":"http","host":"","port":80}`, `{"type":"http","host":"h","port":0}`, `{"type":"http","host":"h","port":80,"bypass":"10.0.0.0/99"}`} {
		proxyCall(t, srv, "PUT", b, 400)
	}
	if outbound.CurrentProxy() != nil {
		t.Fatal("bad input applied")
	}
}

func TestProxySettingsOwnerOnly(t *testing.T) {
	srv, _, _ := proxyServer(t, webserver.RoleAdmin)
	proxyCall(t, srv, "PUT", `{"type":"http","host":"h","port":80}`, 403)
}
