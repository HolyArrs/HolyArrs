package accounts

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestSetupSavesDisplayName(t *testing.T) {
	f := gated(t, Options{})
	c := client(t)
	resp, out := call(t, c, "POST", f.srv.URL+"/api/v1/auth/setup", `{"name":"luis","password":"`+pw+`","display_name":" Luis B "}`, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("setup: %d %s", resp.StatusCode, out)
	}
	var v struct {
		CSRF string `json:"csrf"`
	}
	jsonOf(t, out, &v)
	if _, prof := (person{c, v.CSRF}).do(t, f, "GET", "/api/v1/my/profile", ""); !strings.Contains(prof, `"display_name":"Luis B"`) {
		t.Fatalf("profile: %s", prof)
	}
	f2 := gated(t, Options{})
	if resp, out := call(t, client(t), "POST", f2.srv.URL+"/api/v1/auth/setup", `{"name":"luis","password":"`+pw+`","display_name":"a\u0007b"}`, nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad display name: %d %s", resp.StatusCode, out)
	}
}

func TestSetupLinkJellyfin(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	ms := fakeSetupServer(t)
	body := func(pass string) string {
		b, _ := json.Marshal(map[string]string{"url": ms.URL, "name": "boss", "password": pass})
		return string(b)
	}
	if resp, out := own.do(t, f, "POST", "/api/v1/auth/setup/link/plex", body("right")); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("plex kind: %d %s", resp.StatusCode, out)
	}
	if resp, out := own.do(t, f, "POST", "/api/v1/auth/setup/link/jellyfin", body("wrong")); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong password: %d %s", resp.StatusCode, out)
	}
	if resp, out := own.do(t, f, "POST", "/api/v1/auth/setup/link/jellyfin", body("right")); resp.StatusCode != http.StatusOK || !strings.Contains(out, `"name":"Boss"`) {
		t.Fatalf("link: %d %s", resp.StatusCode, out)
	}
	if _, out := own.do(t, f, "GET", "/api/v1/auth/media-link", ""); !strings.Contains(out, `"provider":"jellyfin"`) {
		t.Fatalf("links: %s", out)
	}
	if resp, out := own.do(t, f, http.MethodPost, "/api/v1/auth/setup/done", ``); resp.StatusCode != http.StatusOK {
		t.Fatalf("setup done: %d %s", resp.StatusCode, out)
	}
	if resp, out := own.do(t, f, "POST", "/api/v1/auth/setup/link/jellyfin", body("right")); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("after setup: %d %s", resp.StatusCode, out)
	}
}
