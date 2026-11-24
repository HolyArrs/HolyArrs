package accounts

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// route: GET /api/v1/auth/plex/servers
func TestPlexServersList(t *testing.T) {
	m := http.NewServeMux()
	m.HandleFunc("POST /api/v2/pins", func(w http.ResponseWriter, _ *http.Request) { _, _ = fmt.Fprint(w, `{"id":901,"code":"LINK"}`) })
	m.HandleFunc("GET /api/v2/pins/{id}", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"authToken": ownerPlexTok})
	})
	m.HandleFunc("GET /api/v2/user", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"id":1,"username":"boss"}`)) })
	m.HandleFunc("GET /api/v2/resources", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Plex-Token") != ownerPlexTok {
			http.Error(w, "no", 401)
			return
		}
		_, _ = w.Write([]byte(`[{"name":"Home","clientIdentifier":"mine-1","provides":"server","owned":true,"connections":[
			{"protocol":"https","address":"203.0.113.5","port":32400,"uri":"https://203-0-113-5.abc.plex.direct:32400","local":false,"relay":false},
			{"protocol":"https","address":"10.0.0.5","port":32400,"uri":"https://10-0-0-5.abc.plex.direct:32400","local":true,"relay":false},
			{"protocol":"http","address":"10.0.0.5","port":32400,"uri":"http://10.0.0.5:32400","local":true,"relay":false},
			{"protocol":"http","address":"203.0.113.5","port":32400,"uri":"http://203.0.113.5:32400","local":false,"relay":false},
			{"protocol":"https","address":"198.51.100.9","port":32400,"local":false,"relay":false},
			{"protocol":"https","address":"1.1.1.1","port":8443,"uri":"https://relay","local":false,"relay":true}]},
			{"name":"Friend","clientIdentifier":"theirs","provides":"server","owned":false,"connections":[{"protocol":"http","address":"9.9.9.9","port":32400,"local":true}]},
			{"name":"Phone","clientIdentifier":"p","provides":"player","owned":true}]`))
	})
	p := httptest.NewServer(m)
	t.Cleanup(p.Close)
	f := gated(t, Options{PlexURL: p.URL})
	own := ownerOf(t, f)
	// Nothing linked yet: no list.
	if resp, _ := own.do(t, f, "GET", "/api/v1/auth/plex/servers", ""); resp.StatusCode != http.StatusConflict {
		t.Fatalf("unlinked: %d", resp.StatusCode)
	}
	own.do(t, f, "POST", "/api/v1/auth/plex/link/start", "")
	if resp, out := own.do(t, f, "POST", "/api/v1/auth/plex/link/finish", `{"id":901}`); resp.StatusCode != 200 {
		t.Fatalf("link: %d %s", resp.StatusCode, out)
	}
	resp, out := own.do(t, f, "GET", "/api/v1/auth/plex/servers", "")
	if resp.StatusCode != 200 {
		t.Fatalf("list: %d %s", resp.StatusCode, out)
	}
	if strings.Contains(out, ownerPlexTok) || strings.Contains(out, "theirs") || strings.Contains(out, "relay") {
		t.Fatalf("leaked or listed too much: %s", out)
	}
	var got struct {
		Servers []plexServerChoice `json:"servers"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil || len(got.Servers) != 2 {
		t.Fatalf("servers: %v %s", err, out)
	}
	// One row per address, local first; plain http only on the local one, and
	// a remote address with no usable https URI is dropped, never made into http.
	l, r := got.Servers[0], got.Servers[1]
	if l.Address != "10.0.0.5" || !l.Local || l.URL != "http://10.0.0.5:32400" || l.SecureURL != "https://10-0-0-5.abc.plex.direct:32400" || l.Name != "Home" || l.ID != "mine-1" {
		t.Fatalf("local: %+v", l)
	}
	if r.Address != "203.0.113.5" || r.Local || r.URL != "https://203-0-113-5.abc.plex.direct:32400" || strings.Contains(out, "http://203.") || strings.Contains(out, "198.51.100.9") || r.SecureURL != "https://203-0-113-5.abc.plex.direct:32400" {
		t.Fatalf("remote: %+v", r)
	}
}

func TestPlexServersPlexDown(t *testing.T) {
	m := http.NewServeMux()
	m.HandleFunc("POST /api/v2/pins", func(w http.ResponseWriter, _ *http.Request) { _, _ = fmt.Fprint(w, `{"id":901,"code":"LINK"}`) })
	m.HandleFunc("GET /api/v2/pins/{id}", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"authToken": ownerPlexTok})
	})
	m.HandleFunc("GET /api/v2/user", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"id":1,"username":"boss"}`)) })
	n := 0
	m.HandleFunc("GET /api/v2/resources", func(w http.ResponseWriter, _ *http.Request) {
		n++
		if n > 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`[]`))
	})
	p := httptest.NewServer(m)
	t.Cleanup(p.Close)
	f := gated(t, Options{PlexURL: p.URL})
	own := ownerOf(t, f)
	own.do(t, f, "POST", "/api/v1/auth/plex/link/start", "")
	own.do(t, f, "POST", "/api/v1/auth/plex/link/finish", `{"id":901}`)
	if resp, _ := own.do(t, f, "GET", "/api/v1/auth/plex/servers", ""); resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("plex down: %d", resp.StatusCode)
	}
}
