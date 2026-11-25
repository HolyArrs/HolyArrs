package accounts

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// serverAccess stands in for the access token plex.tv gives for a server.
const serverAccess = "server-access-value"

// fakePlexTV is a plex.tv that links the owner and lists the given resources.
func fakePlexTV(t *testing.T, resources string) string {
	t.Helper()
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
		_, _ = w.Write([]byte(resources))
	})
	p := httptest.NewServer(m)
	t.Cleanup(p.Close)
	return p.URL
}

// route: GET /api/v1/auth/plex/servers
func TestPlexServersPickTable(t *testing.T) {
	conn := func(proto, addr, uri string, local, relay bool) string {
		return fmt.Sprintf(`{"protocol":%q,"address":%q,"port":32400,"uri":%q,"local":%t,"relay":%t}`, proto, addr, uri, local, relay)
	}
	server := func(name, id string, conns ...string) string {
		return `{"name":"` + name + `","clientIdentifier":"` + id + `","provides":"server","owned":true,"accessToken":"` + serverAccess + `","connections":[` + strings.Join(conns, ",") + `]}`
	}
	cases := []struct {
		name  string
		res   string
		want  []string // each row: id|url|relay
		never []string
	}{
		{
			name: "relay is dropped when a direct address exists",
			res: "[" + server("Home", "a",
				conn("https", "203.0.113.5", "https://203-0-113-5.x.plex.direct:32400", false, false),
				conn("https", "198.51.100.1", "https://198-51-100-1.x.plex.direct:8443", false, true)) + "]",
			want:  []string{"a|https://203-0-113-5.x.plex.direct:32400|false"},
			never: []string{"198-51-100-1"},
		},
		{
			name: "relay is offered when it is the only way in",
			res: "[" + server("Far", "b",
				conn("https", "198.51.100.1", "https://198-51-100-1.x.plex.direct:8443", false, true)) + "]",
			want: []string{"b|https://198-51-100-1.x.plex.direct:8443|true"},
		},
		{
			name: "a plain-http relay is never offered",
			res: "[" + server("Far", "c",
				conn("http", "198.51.100.1", "http://198.51.100.1:8443", false, true)) + "]",
			want: nil,
		},
		{
			name: "two servers keep local before remote and relay last",
			res: "[" + server("Cabin", "r", conn("https", "198.51.100.1", "https://198-51-100-1.x.plex.direct:8443", false, true)) + "," +
				server("Home", "h", conn("http", "10.0.0.5", "http://10.0.0.5:32400", true, false)) + "]",
			want: []string{"h|http://10.0.0.5:32400|false", "r|https://198-51-100-1.x.plex.direct:8443|true"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := gated(t, Options{PlexURL: fakePlexTV(t, tc.res)})
			own := ownerOf(t, f)
			own.do(t, f, "POST", "/api/v1/auth/plex/link/start", "")
			if resp, out := own.do(t, f, "POST", "/api/v1/auth/plex/link/finish", `{"id":901}`); resp.StatusCode != 200 {
				t.Fatalf("link: %d %s", resp.StatusCode, out)
			}
			resp, out := own.do(t, f, "GET", "/api/v1/auth/plex/servers", "")
			if resp.StatusCode != 200 {
				t.Fatalf("list: %d %s", resp.StatusCode, out)
			}
			for _, bad := range append(tc.never, ownerPlexTok, serverAccess) {
				if strings.Contains(out, bad) {
					t.Fatalf("list shows %q: %s", bad, out)
				}
			}
			var got struct {
				Servers []plexServerChoice `json:"servers"`
			}
			jsonOf(t, out, &got)
			rows := []string{}
			for _, s := range got.Servers {
				rows = append(rows, fmt.Sprintf("%s|%s|%t", s.ID, s.URL, s.Relay))
			}
			if strings.Join(rows, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("rows %v, want %v", rows, tc.want)
			}
		})
	}
}

// An API key, even the owner's own, never lists the owner's Plex servers:
// only a browser session does.
func TestPlexServersSessionOnly(t *testing.T) {
	f := gated(t, Options{PlexURL: fakePlexTV(t, `[]`)})
	own := ownerOf(t, f)
	own.do(t, f, "POST", "/api/v1/auth/plex/link/start", "")
	own.do(t, f, "POST", "/api/v1/auth/plex/link/finish", `{"id":901}`)
	for _, scopes := range []string{`"admin"`, `"all"`} {
		key := keyWith(t, f, own, scopes)
		resp, out := call(t, client(t), "GET", f.srv.URL+"/api/v1/auth/plex/servers", "", map[string]string{APIKeyHeader: key})
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("key %s listed servers: %d %s", scopes, resp.StatusCode, out)
		}
	}
	member := join(t, f, invite(t, f, own, `{}`), "pat")
	if resp, _ := member.do(t, f, "GET", "/api/v1/auth/plex/servers", ""); resp.StatusCode/100 == 2 {
		t.Fatalf("member listed servers: %d", resp.StatusCode)
	}
}
