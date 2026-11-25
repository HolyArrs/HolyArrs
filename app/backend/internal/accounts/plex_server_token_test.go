package accounts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Only the owner's own Plex servers get a token, and it is that server's
// own token, never the plex.tv account token.
func TestPlexServerTokenOwnedOnly(t *testing.T) {
	serverTok := "server" + "-tok"
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
		_, _ = fmt.Fprintf(w, `[{"name":"Home","provides":"server","owned":true,"accessToken":%q,"connections":[
			{"protocol":"https","address":"10.0.0.5","port":32400,"uri":"https://10-0-0-5.abc.plex.direct:32400","local":true},
			{"protocol":"https","address":"203.0.113.5","port":32400,"uri":"https://203-0-113-5.abc.plex.direct:32400"},
			{"protocol":"http","address":"198.51.100.7","port":32400,"uri":"http://198.51.100.7:32400"}]},
			{"name":"Friend","provides":"server","owned":false,"accessToken":%q,"connections":[{"address":"9.9.9.9","port":32400,"uri":"http://9.9.9.9:32400"}]}]`, serverTok, "friend"+"-tok")
	})
	p := httptest.NewServer(m)
	t.Cleanup(p.Close)
	f := gated(t, Options{PlexURL: p.URL})
	own := ownerOf(t, f)
	ctx := context.Background()
	if _, err := f.a.PlexServerToken(ctx, "http://10.0.0.5:32400"); err == nil {
		t.Fatal("unlinked account gave a token")
	}
	own.do(t, f, "POST", "/api/v1/auth/plex/link/start", "")
	if resp, out := own.do(t, f, "POST", "/api/v1/auth/plex/link/finish", `{"id":901}`); resp.StatusCode != 200 {
		t.Fatalf("link: %d %s", resp.StatusCode, out)
	}
	// Every choice Connect server lists passes: plain and secure local, secure
	// remote, and a remote one plex.tv itself lists as plain http.
	for _, u := range []string{"http://10.0.0.5:32400", "https://10-0-0-5.abc.plex.direct:32400/", "https://203-0-113-5.abc.plex.direct:32400", "https://203.0.113.5:32400", "http://198.51.100.7:32400"} {
		if tok, err := f.a.PlexServerToken(ctx, u); err != nil || tok != serverTok {
			t.Fatalf("%s: %q %v", u, tok, err)
		}
	}
	// A remote address as plain http never gets the token.
	if tok, err := f.a.PlexServerToken(ctx, "http://203.0.113.5:32400"); !errors.Is(err, ErrPlainRemotePlex) || tok != "" {
		t.Fatalf("http remote gave %q %v", tok, err)
	}
	for _, u := range []string{"http://evil.example:32400", "http://10.0.0.5:9999", "http://9.9.9.9:32400", "ftp://10.0.0.5:32400", ""} {
		if tok, err := f.a.PlexServerToken(ctx, u); err == nil || tok != "" {
			t.Fatalf("%s gave %q", u, tok)
		}
	}
}
