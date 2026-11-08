//go:build labms

package accounts

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

func TestAcceptanceLabJellyfinSignInMakesMemberAndEndsOutsideSession(t *testing.T) {
	serverURL := os.Getenv("HOLYARRS_LAB_JELLYFIN_URL")
	username := os.Getenv("HOLYARRS_LAB_JELLYFIN_USERNAME")
	password := os.Getenv("HOLYARRS_LAB_JELLYFIN_PASSWORD")
	if serverURL == "" || username == "" || password == "" {
		t.Skip("the opt-in Jellyfin lab environment is not set")
	}

	target, err := url.Parse(serverURL)
	if err != nil || target.Scheme != "http" || target.Host == "" {
		t.Fatal("the Jellyfin lab address is invalid")
	}
	allowed := os.Getenv("HOLYARRS_LAB_HOST")
	if allowed == "" {
		allowed = target.Hostname()
	}
	if host := target.Hostname(); host != "127.0.0.1" && host != allowed {
		t.Fatalf("the Jellyfin acceptance target is not an allowed lab host: %q", host)
	}
	logOutput := log.Writer()
	log.SetOutput(io.Discard)
	t.Cleanup(func() { log.SetOutput(logOutput) })

	var authenticationCalls atomic.Int32
	var logoutRequests atomic.Int32
	var completedLogouts atomic.Int32
	var proxyErrors atomic.Int32
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		proxyErrors.Add(1)
		http.Error(w, "the disposable Jellyfin lab did not answer", http.StatusBadGateway)
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		r := resp.Request
		if r.Method == http.MethodPost && r.URL.Path == "/Sessions/Logout" && resp.StatusCode/100 == 2 &&
			strings.Contains(r.Header.Get("Authorization"), `Token="`) && r.Header.Get("X-Emby-Token") != "" {
			completedLogouts.Add(1)
		}
		return nil
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/Users/AuthenticateByName":
			authenticationCalls.Add(1)
		case r.Method == http.MethodPost && r.URL.Path == "/Sessions/Logout":
			if strings.Contains(r.Header.Get("Authorization"), `Token="`) && r.Header.Get("X-Emby-Token") != "" {
				logoutRequests.Add(1)
			}
		}
		proxy.ServeHTTP(w, r)
	}))
	t.Cleanup(upstream.Close)

	f := gated(t, Options{})
	owner := ownerOf(t, f)
	settings, _ := json.Marshal(map[string]any{
		"jellyfin": map[string]any{"on": true, "url": upstream.URL, "join": JoinEveryone},
	})
	if resp, _ := owner.do(t, f, http.MethodPut, "/api/v1/auth/settings", string(settings)); resp.StatusCode != http.StatusOK {
		t.Fatalf("enable Jellyfin sign-in: status %d", resp.StatusCode)
	}

	credentials, _ := json.Marshal(map[string]string{"name": username, "password": password})
	member := client(t)
	resp, _ := call(t, member, http.MethodPost, f.srv.URL+"/api/v1/auth/media-server/jellyfin", string(credentials), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Jellyfin sign-in: status %d", resp.StatusCode)
	}

	resp, body := call(t, member, http.MethodGet, f.srv.URL+"/api/v1/auth/me", "", nil)
	var me struct {
		Name string `json:"name"`
		Role string `json:"role"`
	}
	if resp.StatusCode != http.StatusOK || json.Unmarshal([]byte(body), &me) != nil || me.Name == "" || me.Role != webserver.RoleMember {
		t.Fatalf("the outside sign-in did not create a signed-in member: status=%d role=%q", resp.StatusCode, me.Role)
	}

	var name, role, state, passwordHash, externalID string
	err = f.s.Read().QueryRow(`SELECT u.name, u.role, u.state, u.password_hash, l.external_id
		FROM user_links l JOIN users u ON u.id = l.user_id WHERE l.provider = ?`, ProviderJellyfin).
		Scan(&name, &role, &state, &passwordHash, &externalID)
	if err != nil || name != me.Name || role != webserver.RoleMember || state != "active" || passwordHash != "" || externalID == "" {
		t.Fatalf("the Jellyfin link was not stored as an outside-only member: query_error=%v role=%q state=%q", err, role, state)
	}
	if authenticationCalls.Load() != 1 || logoutRequests.Load() != 1 || completedLogouts.Load() != 1 || proxyErrors.Load() != 0 {
		t.Fatalf("unexpected Jellyfin calls: authenticate=%d logout_request=%d completed_logout=%d proxy_errors=%d",
			authenticationCalls.Load(), logoutRequests.Load(), completedLogouts.Load(), proxyErrors.Load())
	}
}
