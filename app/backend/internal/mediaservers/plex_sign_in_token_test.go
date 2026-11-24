package mediaservers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// A Plex server token from the owner's Plex sign-in reaches only the
// owner's own Plex server: never a non-owner's request, never a server whose
// kind is not exactly plex, never a foreign address, and never the plex.tv
// account token itself.
func TestPlexSignInTokenOnlyOwnedServers(t *testing.T) {
	const accountTok, serverTok = "ACCOUNT-TOKEN", "SERVER-TOKEN"
	var mu sync.Mutex
	seen := []string{}
	record := func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Header.Get("X-Plex-Token")+r.URL.Query().Get("X-Plex-Token"))
		mu.Unlock()
		w.WriteHeader(http.StatusUnauthorized)
	}
	attacker := httptest.NewServer(http.HandlerFunc(record))
	t.Cleanup(attacker.Close)
	owned := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) }))
	t.Cleanup(owned.Close)

	w := newWorld(t)
	ctx := context.Background()
	if err := w.v.SetSetting(ctx, "this_app", "sign_in_plex_owner", accountTok, database.System); err != nil {
		t.Fatal(err)
	}
	h := &Handlers{Store: w.s, Vault: w.v, Linker: &Linker{Store: w.s, Vault: w.v},
		// Stands in for plex.tv: only the owned server's address matches.
		PlexServerToken: func(_ context.Context, u string) (string, error) {
			if strings.TrimRight(u, "/") == owned.URL {
				return serverTok, nil
			}
			return "", errors.New("not owned")
		}}
	mux := http.NewServeMux()
	h.Routes(mux)
	as := func(c *webserver.Caller) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			if c != nil {
				r = r.WithContext(webserver.ContextWith(r.Context(), *c))
			}
			mux.ServeHTTP(rw, r)
		})
	}
	owner := &webserver.Caller{UserID: 1, Via: "session", Role: webserver.RoleOwner}
	admin := &webserver.Caller{UserID: 2, Via: "session", Role: webserver.RoleAdmin}
	stored := func(t *testing.T, body string, c *webserver.Caller) string {
		t.Helper()
		rec := httptest.NewRecorder()
		as(c).ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/media-servers", strings.NewReader(body)))
		ms, err := listServers(ctx, w.s.Read())
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range ms {
			if strings.Contains(body, `"`+m.Name+`"`) && m.TokenSecretID != nil {
				tok, _ := w.v.Get(ctx, *m.TokenSecretID)
				return tok
			}
		}
		return ""
	}
	body := func(name, kind, u string) string {
		return `{"name":"` + name + `","kind":"` + kind + `","url":"` + u + `","allow_local_network":true,"use_plex_sign_in":true}`
	}
	if tok := stored(t, body("Admin try", "plex", owned.URL), admin); tok != "" {
		t.Fatalf("non-owner got %q", tok)
	}
	if tok := stored(t, body("No kind", "", owned.URL), owner); tok != "" {
		t.Fatalf("empty kind got %q", tok)
	}
	if tok := stored(t, body("Jelly", "jellyfin", owned.URL), owner); tok != "" {
		t.Fatalf("jellyfin got %q", tok)
	}
	if tok := stored(t, body("Foreign", "plex", attacker.URL), owner); tok != "" {
		t.Fatalf("foreign address got %q", tok)
	}
	if tok := stored(t, body("Mine", "plex", owned.URL), owner); tok != serverTok {
		t.Fatalf("owned server got %q", tok)
	}
	// Editing the owned server over to a foreign address gets no token either.
	ms, _ := listServers(ctx, w.s.Read())
	for _, m := range ms {
		if m.Name != "Mine" {
			continue
		}
		rec := httptest.NewRecorder()
		as(owner).ServeHTTP(rec, httptest.NewRequest("PUT", "/api/v1/media-servers/"+strconv.FormatInt(m.ID, 10), strings.NewReader(body("Mine", "plex", attacker.URL))))
		if rec.Code/100 == 2 {
			t.Fatalf("edit to a foreign address kept going: %d %s", rec.Code, rec.Body)
		}
		got, _ := getServer(ctx, w.s.Read(), m.ID)
		if got.URL != m.URL {
			t.Fatalf("edit moved the server to %s", got.URL)
		}
	}
	// Whatever was saved, testing every server sends the attacker no token.
	ms, _ = listServers(ctx, w.s.Read())
	for _, m := range ms {
		rec := httptest.NewRecorder()
		as(owner).ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/media-servers/"+strconv.FormatInt(m.ID, 10)+"/test", nil))
	}
	mu.Lock()
	defer mu.Unlock()
	for _, s := range seen {
		if s != "" {
			t.Fatalf("attacker received a token: %q", s)
		}
	}
}
