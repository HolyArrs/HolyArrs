package accounts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSSOEnvironmentFailsClosedWithoutLeakingSecret(t *testing.T) {
	t.Setenv("HOLYARRS_OIDC_ENABLED", "true")
	t.Setenv("HOLYARRS_OIDC_ISSUER", "https://id.example.test")
	t.Setenv("HOLYARRS_OIDC_CLIENT_ID", "holyarrs")
	t.Setenv("HOLYARRS_OIDC_REDIRECT_URL", "https://app.example.test/api/v1/auth/oidc/callback")
	p := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(p, []byte("synthetic-client-value"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOLYARRS_OIDC_CLIENT_SECRET_FILE", p)
	opt, err := SSOFromEnv("")
	if err != nil || opt.OIDC == nil || opt.OIDC.ClientSecret != "synthetic-client-value" || opt.OIDC.AllowJoin {
		t.Fatal("valid enabled OIDC configuration refused or join enabled")
	}
	if _, err := SSOFromEnv("/configured-base"); err == nil {
		t.Fatal("callback URL-base mismatch was accepted at startup")
	}
	t.Setenv("HOLYARRS_OIDC_REDIRECT_URL", "https://app.example.test/configured-base/api/v1/auth/oidc/callback")
	if _, err := SSOFromEnv("/configured-base"); err != nil {
		t.Fatal("matching callback URL base was refused at startup")
	}
	t.Setenv("HOLYARRS_OIDC_REDIRECT_URL", "https://bad.example.test/unsafe?secret=synthetic-client-value")
	_, err = SSOFromEnv("")
	if err == nil || strings.Contains(err.Error(), "synthetic-client-value") {
		t.Fatal("invalid config accepted or leaked private material")
	}
}

func TestForwardConfigRequiresExplicitPeers(t *testing.T) {
	for _, peers := range [][]string{nil, {""}, {"0.0.0.0/0"}, {"::/0"}, {"proxy.example.test"}} {
		if _, err := newForwardRuntime(ForwardAuthOptions{Issuer: "https://proxy.example.test", TrustedPeers: peers}); err == nil {
			t.Fatal("unbounded or absent direct-peer trust accepted")
		}
	}
	if _, err := newForwardRuntime(ForwardAuthOptions{Issuer: "https://proxy.example.test", TrustedPeers: []string{"10.2.3.4", "::1"}}); err != nil {
		t.Fatal("explicit peers refused")
	}
}

func TestInvalidDirectSSOOptionsDisableEveryProvider(t *testing.T) {
	for _, options := range []SSOOptions{{OIDC: &OIDCOptions{}}, {Forward: &ForwardAuthOptions{}}} {
		f := setup(t, Options{SSO: options})
		c := client(t)
		csrf := setupOwner(t, f, c)
		if f.a.sso.Load() == nil || f.a.ssoNow().oidc != nil || f.a.ssoNow().forward != nil {
			t.Fatal("invalid direct options did not disable SSO")
		}
		for _, path := range []string{"oidc/start", "oidc/finish", "oidc/link", "oidc/link-finish", "forward/login", "forward/link"} {
			resp, _ := call(t, c, "POST", f.srv.URL+"/api/v1/auth/"+path, "", map[string]string{CSRFHeader: csrf})
			if resp.StatusCode != 404 {
				t.Fatalf("disabled %s status = %d", path, resp.StatusCode)
			}
		}
		resp, _ := call(t, c, "GET", f.srv.URL+"/api/v1/auth/oidc/callback", "", nil)
		if resp.StatusCode != 400 {
			t.Fatalf("disabled callback status = %d", resp.StatusCode)
		}
		f.a.Close()
		f.a.Close()
	}
}
