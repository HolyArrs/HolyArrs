package accounts

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func forwardOptions(join bool) Options {
	return Options{SSO: SSOOptions{Forward: &ForwardAuthOptions{Issuer: "https://proxy.example.test", TrustedPeers: []string{"127.0.0.1/32"}, AllowJoin: join}}}
}

func TestForwardAuthDirectPeerAndLeastPrivilege(t *testing.T) {
	f := gated(t, forwardOptions(true))
	ownerOf(t, f)
	c := client(t)
	h := map[string]string{"Remote-User": "stable-user", "Remote-Name": "luis", "Remote-Role": "owner", "X-Forwarded-For": "203.0.113.1"}
	resp, body := call(t, c, "POST", f.srv.URL+"/api/v1/auth/forward/login", "", h)
	if resp.StatusCode != 200 {
		t.Fatalf("trusted direct peer refused: %d %s", resp.StatusCode, body)
	}
	resp, body = call(t, c, "GET", f.srv.URL+"/api/v1/auth/me", "", nil)
	if resp.StatusCode != 200 || !strings.Contains(body, `"role":"member"`) || !strings.Contains(body, `"perms":["ask"]`) {
		t.Fatalf("proxy role/name elevated user: %d %s", resp.StatusCode, body)
	}
	f.a.ssoNow().forward.peers = nil
	resp, _ = call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/forward/login", "", map[string]string{"Remote-User": "other", "X-Forwarded-For": "127.0.0.1"})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("forwarded-for established trust: %d", resp.StatusCode)
	}
}

func TestForwardAuthDisabledOwnerAndHeaderFailures(t *testing.T) {
	f := gated(t, Options{})
	resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/forward/login", "", map[string]string{"Remote-User": "x"})
	if resp.StatusCode != 404 {
		t.Fatalf("disabled provider accepted: %d", resp.StatusCode)
	}
	f = gated(t, forwardOptions(true))
	resp, _ = call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/forward/login", "", map[string]string{"Remote-User": "x"})
	if resp.StatusCode != 403 {
		t.Fatalf("provider created first owner: %d", resp.StatusCode)
	}
	ownerOf(t, f)
	for _, subject := range []string{"", " ", strings.Repeat("x", 256), "two,users"} {
		resp, _ = call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/forward/login", "", map[string]string{"Remote-User": subject})
		if resp.StatusCode != 403 {
			t.Fatalf("invalid subject accepted: %d", resp.StatusCode)
		}
	}
}

func TestForwardAuthExplicitLinkAndAccountCollision(t *testing.T) {
	f := gated(t, forwardOptions(false))
	own := ownerOf(t, f)
	headers := map[string]string{"Remote-User": "owner-subject", CSRFHeader: own.csrf}
	resp, body := call(t, own.c, "POST", f.srv.URL+"/api/v1/auth/forward/link", "", headers)
	if resp.StatusCode != 200 {
		t.Fatalf("explicit link: %d %s", resp.StatusCode, body)
	}
	resp, _ = call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/forward/login", "", map[string]string{"Remote-User": "unlinked"})
	if resp.StatusCode != 403 {
		t.Fatalf("join-disabled identity accepted: %d", resp.StatusCode)
	}
	resp, _ = call(t, own.c, "POST", f.srv.URL+"/api/v1/auth/forward/link", "", map[string]string{"Remote-User": "attacker"})
	if resp.StatusCode != 403 {
		t.Fatalf("link without CSRF: %d", resp.StatusCode)
	}
	id, _, err := f.a.ssoIdentity(context.Background(), "forward_auth", "https://proxy.example.test", "another", "another", true, 0)
	if err != nil || id == 0 {
		t.Fatal("fixture identity failed")
	}
	headers["Remote-User"] = "another"
	resp, _ = call(t, own.c, "POST", f.srv.URL+"/api/v1/auth/forward/link", "", headers)
	if resp.StatusCode != 403 {
		t.Fatalf("linked identity reassigned: %d", resp.StatusCode)
	}
}
