package requests

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// fakePrivateTrakt serves kim's private watchlist only with the access
// token it handed out at sign-in; good says whether Trakt still takes it.
func fakePrivateTrakt(t *testing.T, good *atomic.Bool, auths *[]string) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/users/kim/") {
			*auths = append(*auths, r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/oauth/device/code":
			_ = json.NewEncoder(w).Encode(map[string]any{"device_code": "dev", "user_code": "CODE", "verification_url": "https://trakt.tv/activate", "expires_in": 600, "interval": 1})
			return
		case "/oauth/device/token":
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok-ok", "refresh_token": "ref-ok", "expires_in": 86400})
			return
		case "/users/settings":
			_, _ = w.Write([]byte(`{"user":{"username":"kim"}}`))
			return
		}
		if r.Header.Get("trakt-api-key") != fakeTraktClient {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		auth := r.Header.Get("Authorization")
		switch {
		case auth == "":
			w.WriteHeader(http.StatusNotFound) // private: hidden without the sign-in
			return
		case auth != "Bearer tok-ok" || !good.Load():
			w.WriteHeader(http.StatusUnauthorized)
			return
		case r.URL.Path != "/users/kim/watchlist/movies":
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("X-Pagination-Page-Count", "1")
		_ = json.NewEncoder(w).Encode([]map[string]any{{"type": "movie", "movie": map[string]any{"title": "Quiet Orchard", "ids": map[string]any{"tmdb": 900003}}}})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestSync_PrivateTraktListWithSignIn: signed in with Trakt, a private
// watchlist is read with the access token. When Trakt refuses the token
// (revoked), the read fails saying to sign in again, the sign-in ends, and
// the next read is a plain public one that cannot see the private list.
func TestSync_PrivateTraktListWithSignIn(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	good := &atomic.Bool{}
	good.Store(true)
	var auths []string
	srv := fakePrivateTrakt(t, good, &auths)
	v, err := secrets.Open(ctx, e.s, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	by := database.Actor{Kind: "user", ID: 1}
	auth := &metadata.TraktAuth{Secrets: v, Base: srv.URL}
	if err := v.SetSetting(ctx, metadata.TraktKeyGroup, metadata.TraktKeyName, fakeTraktClient, by); err != nil {
		t.Fatal(err)
	}
	if err := auth.SetSecret(ctx, "secret", by); err != nil {
		t.Fatal(err)
	}
	e.svc.TraktBase, e.svc.TraktAuth = srv.URL, auth
	owner := domain.Actor{Kind: "user", ID: 1}
	l, _ := AddOutsideList(ctx, e.s, OutsideListInput{Name: "Mine", Kind: ListTrakt, Ref: "kim/watchlist"}, owner)
	// Not signed in: the private watchlist is not there.
	if _, err := e.svc.Sync(ctx, l.ID, domain.System); err == nil || !strings.Contains(err.Error(), "private") {
		t.Fatalf("public read of a private list: %v", err)
	}
	if _, err := auth.StartSignIn(ctx, 1, by); err != nil { // the owner (person 1) signs in
		t.Fatal(err)
	}
	if s, err := auth.CheckSignIn(ctx); s != metadata.TraktSignedIn || err != nil {
		t.Fatalf("sign-in %q %v", s, err)
	}
	if res, err := e.svc.Sync(ctx, l.ID, domain.System); err != nil || res.Added != 1 {
		t.Fatalf("signed-in read %+v %v", res, err)
	}
	if auths[len(auths)-1] != "Bearer tok-ok" {
		t.Fatalf("the owner's list was read without the sign-in: %q", auths)
	}
	// Someone else's list, or the owner's list last changed by someone
	// else, is read with the key alone: never with the owner's sign-in.
	other := domain.Actor{Kind: "user", ID: 2}
	theirs, _ := AddOutsideList(ctx, e.s, OutsideListInput{Name: "Theirs", Kind: ListTrakt, Ref: "kim/watchlist"}, other)
	n := len(auths)
	if _, err := e.svc.Sync(ctx, theirs.ID, domain.System); err == nil || !strings.Contains(err.Error(), "private") {
		t.Fatalf("another person's list read the private list: %v", err)
	}
	mine2, _ := AddOutsideList(ctx, e.s, OutsideListInput{Name: "Mine 2", Kind: ListTrakt, Ref: "kim/favourites"}, owner)
	if _, err := EditOutsideList(ctx, e.s, mine2.ID, OutsideListInput{Name: "Mine 2", Kind: ListTrakt, Ref: "kim/watchlist"}, other); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Sync(ctx, mine2.ID, domain.System); err == nil {
		t.Fatal("a list changed by someone else was read with the sign-in")
	}
	for _, a := range auths[n:] {
		if a != "" {
			t.Fatalf("a list not the signed-in person's sent %q", a)
		}
	}
	good.Store(false)
	if _, err := e.svc.Sync(ctx, l.ID, domain.System); !errors.Is(err, metadata.ErrTraktSignedOut) {
		t.Fatalf("revoked token: %v", err)
	}
	got, _ := GetOutsideList(ctx, e.s.Read(), l.ID)
	if !strings.Contains(got.LastError, "sign in with Trakt again") || strings.Contains(got.LastError, "tok-ok") {
		t.Fatalf("list error %q", got.LastError)
	}
	if st := auth.Status(ctx); st.SignedIn {
		t.Fatalf("still signed in after a refused token: %+v", st)
	}
	if _, err := e.svc.Sync(ctx, l.ID, domain.System); err == nil || errors.Is(err, metadata.ErrTraktSignedOut) {
		t.Fatalf("read after sign-out: %v", err)
	}
}

// TestTraktSignInRoutes: the sign-in routes answer the status without any
// secret, need a client id, hand out a device code to type at
// trakt.tv/activate, and stop waiting once the code is approved.
func TestTraktSignInRoutes(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	v, err := secrets.Open(ctx, e.s, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	var good atomic.Bool
	var auths []string
	srv := fakePrivateTrakt(t, &good, &auths)
	h := &OutsideListHandlers{Store: e.s, Queue: e.q, Secrets: v, TraktAuth: &metadata.TraktAuth{Secrets: v, Base: srv.URL},
		Actor: func(*http.Request) domain.Actor { return domain.Actor{Kind: "user", ID: 1} }}
	changed := 0
	h.TraktChanged = func() { changed++ }
	m := http.NewServeMux()
	h.TraktKeyRoutes(m)
	h.TraktSignInRoutes(m)
	call := func(method, path, body string) (int, string) {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		w := httptest.NewRecorder()
		m.ServeHTTP(w, r)
		return w.Code, w.Body.String()
	}
	if code, body := call("POST", "/api/v1/settings/trakt-signin", ""); code != http.StatusConflict || !strings.Contains(body, "client id") {
		t.Fatalf("sign-in with no client id: %d %s", code, body)
	}
	if code, body := call("POST", "/api/v1/settings/trakt-signin/check", ""); code != http.StatusConflict {
		t.Fatalf("check with no sign-in: %d %s", code, body)
	}
	if code, _ := call("PUT", "/api/v1/settings/trakt-key", `{"key":"cid-123456789"}`); code != http.StatusOK {
		t.Fatalf("key %d", code)
	}
	if code, body := call("PUT", "/api/v1/settings/trakt-secret", `{"secret":"sec-123"}`); code != http.StatusOK || strings.Contains(body, "sec-123") || !strings.Contains(body, `"client_secret":true`) {
		t.Fatalf("secret %d %s", code, body)
	}
	if code, body := call("PUT", "/api/v1/settings/trakt-secret", `{"secret":""}`); code != http.StatusOK || !strings.Contains(body, `"client_secret":false`) {
		t.Fatalf("secret removed %d %s", code, body)
	}
	if code, body := call("POST", "/api/v1/settings/trakt-signin", ""); code != http.StatusConflict || !strings.Contains(body, "client secret") {
		t.Fatalf("sign-in with no client secret: %d %s", code, body)
	}
	if code, _ := call("PUT", "/api/v1/settings/trakt-secret", `{"secret":"sec-123"}`); code != http.StatusOK {
		t.Fatalf("secret again %d", code)
	}
	code, body := call("POST", "/api/v1/settings/trakt-signin", "")
	if code != http.StatusOK || !strings.Contains(body, `"user_code":"CODE"`) || !strings.Contains(body, `"verification_url":"https://trakt.tv/activate"`) {
		t.Fatalf("start %d %s", code, body)
	}
	code, body = call("GET", "/api/v1/settings/trakt-signin", "")
	if code != http.StatusOK || !strings.Contains(body, `"waiting":true`) || !strings.Contains(body, `"signed_in":false`) || strings.Contains(body, "cid-123456789") {
		t.Fatalf("status %d %s", code, body)
	}
	if changed != 0 {
		t.Fatalf("watch history read asked for %d times before the sign-in ended", changed)
	}
	code, body = call("POST", "/api/v1/settings/trakt-signin/check", "")
	if code != http.StatusOK || !strings.Contains(body, `"state":"signed_in"`) || !strings.Contains(body, `"waiting":false`) || !strings.Contains(body, `"username":"kim"`) {
		t.Fatalf("check %d %s", code, body)
	}
	if strings.Contains(body, "tok-ok") || strings.Contains(body, "ref-ok") {
		t.Fatalf("check shows a token: %s", body)
	}
	code, body = call("GET", "/api/v1/settings/trakt-signin", "")
	if code != http.StatusOK || !strings.Contains(body, `"waiting":false`) || !strings.Contains(body, `"signed_in":true`) {
		t.Fatalf("status after sign-in %d %s", code, body)
	}
	// Signing in reads the person's Trakt history now, not at the next
	// half-hourly read; signing out drops it now too.
	if changed != 1 {
		t.Fatalf("watch history read asked for %d times after the sign-in, want 1", changed)
	}
	if code, _ := call("DELETE", "/api/v1/settings/trakt-signin", ""); code != http.StatusOK {
		t.Fatalf("sign out %d", code)
	}
	if changed != 2 {
		t.Fatalf("watch history read asked for %d times after the sign-out, want 2", changed)
	}
}
