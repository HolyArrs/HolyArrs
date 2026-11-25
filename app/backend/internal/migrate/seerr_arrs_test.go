package migrate

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/migrate/secretsfile"
)

// seerrWithArrs is a small Overseerr that keeps the given Sonarr and Radarr
// servers (with their keys), like GET /api/v1/settings/sonarr|radarr.
func seerrWithArrs(t *testing.T, key string, sonarr, radarr []any) string {
	t.Helper()
	empty := map[string]any{"pageInfo": map[string]any{"pages": 0, "page": 1, "results": 0}, "results": []any{}}
	srv := fakeAPI(t, "X-API-Key", key, map[string]any{
		"/api/v1/status":          map[string]any{"version": "1.34.0"},
		"/api/v1/user":            empty,
		"/api/v1/request":         empty,
		"/api/v1/settings/main":   map[string]any{"apiKey": key},
		"/api/v1/settings/sonarr": sonarr,
		"/api/v1/settings/radarr": radarr,
	})
	return srv.URL
}

func arrEntry(raw, key string) map[string]any {
	u, _ := url.Parse(raw)
	port, _ := strconv.Atoi(u.Port())
	return map[string]any{"name": "Radarr", "hostname": u.Hostname(), "port": port, "useSsl": false, "baseUrl": "", "apiKey": key, "is4k": false}
}

// The Sonarr and Radarr servers Overseerr keeps are listed with its keys,
// and an entry with no usable address is left out.
func TestSeerrArrsListsServersWithKeys(t *testing.T) {
	ru, rk := radarrAPIKeyed(t, "S3CR3T_RADARR_K", nil)
	su := seerrWithArrs(t, "S3CR3T_SEERR_K", []any{map[string]any{"hostname": "", "port": 8989, "apiKey": "x"}}, []any{arrEntry(ru, rk)})
	got, err := SeerrArrs(context.Background(), Source{Kind: Overseerr, URL: su, Key: "S3CR3T_SEERR_K", AllowLocal: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Kind != Radarr || got[0].Key != rk || !got[0].AllowLocal || InstanceAddress(got[0].URL) != InstanceAddress(ru) {
		t.Fatalf("servers %+v, want only Radarr with its key", got)
	}
	// Any other app has none.
	if got, err := SeerrArrs(context.Background(), Source{Kind: Sonarr, URL: su}); err != nil || len(got) != 0 {
		t.Fatalf("sonarr: %+v %v", got, err)
	}
}

// Reading Overseerr also reads the Radarr it keeps: its snapshot comes back
// beside Overseerr's, its key goes to the vault and never to the browser.
func TestReadOverseerrReadsItsArrs(t *testing.T) {
	ctx := context.Background()
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	v, err := secrets.Open(ctx, st, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	r := webserver.NewRouter()
	h := &Handlers{Store: st, Secrets: v, Mover: func() *Mover { return nil }, SafeMode: func() bool { return false }}
	h.Routes(r)
	owner := webserver.ContextWith(ctx, webserver.Caller{UserID: 1, Name: "owner", Role: webserver.RoleOwner, Via: "session"})

	ru, rk := radarrAPIKeyed(t, "S3CR3T_RADARR_K", nil)
	su := seerrWithArrs(t, "S3CR3T_SEERR_K", []any{}, []any{arrEntry(ru, rk), arrEntry("http://127.0.0.1:1", "S3CR3T_GONE")})
	body := `{"kind":"overseerr","url":"` + su + `","key":"S3CR3T_SEERR_K","allow_local_network":true}`
	req := httptest.NewRequest("POST", "/api/v1/migrate/read", strings.NewReader(body)).WithContext(owner)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("read: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), secretMark) {
		t.Fatalf("a key reached the answer: %s", rec.Body.String())
	}
	var out struct {
		Snapshot Saved
		Linked   []struct {
			Kind     Kind  `json:"kind"`
			Snapshot Saved `json:"snapshot"`
		} `json:"linked"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Linked) != 1 || out.Linked[0].Kind != Radarr || out.Linked[0].Snapshot.ID == 0 || out.Linked[0].Snapshot.ID == out.Snapshot.ID {
		t.Fatalf("linked %+v, want Radarr read (the one that does not answer left out)", out.Linked)
	}
	c, _, ok, err := secretsfile.ReadMoveCredentialAt(ctx, st, v, "radarr", ru)
	if err != nil || !ok || c.APIKey != rk {
		t.Fatalf("radarr key kept: ok=%v err=%v", ok, err)
	}
}

// A Seerr read lists the Sonarr or Radarr it could not reach as unread and
// skips the addresses already read; reading that app at a new address with
// linked_from uses the key Seerr keeps, which never reaches the browser.
func TestSeerrUnreadThenLinkedFrom(t *testing.T) {
	ctx := context.Background()
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	v, err := secrets.Open(ctx, st, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	r := webserver.NewRouter()
	h := &Handlers{Store: st, Secrets: v, Mover: func() *Mover { return nil }, SafeMode: func() bool { return false }}
	h.Routes(r)
	owner := webserver.ContextWith(ctx, webserver.Caller{UserID: 1, Name: "owner", Role: webserver.RoleOwner, Via: "session"})
	post := func(path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest("POST", path, strings.NewReader(body)).WithContext(owner))
		if strings.Contains(rec.Body.String(), secretMark) {
			t.Fatalf("a key reached the answer: %s", rec.Body.String())
		}
		return rec
	}

	ru, rk := radarrAPIKeyed(t, "S3CR3T_RADARR_K", nil)
	// Seerr keeps Radarr under a name only Docker resolves, and a second one already read.
	other, ok2 := radarrAPIKeyed(t, "S3CR3T_RADARR_2", nil)
	gone := "http://127.0.0.1:1"
	su := seerrWithArrs(t, "S3CR3T_SEERR_K", []any{}, []any{arrEntry(gone, rk), arrEntry(other, ok2)})
	rec := post("/api/v1/migrate/read", `{"kind":"seerr","url":"`+su+`","key":"S3CR3T_SEERR_K","allow_local_network":true,"skip_linked":["`+other+`"]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("read: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Linked  []struct{ Kind Kind } `json:"linked"`
		Unread  []LinkedUnread        `json:"unread"`
		Skipped []LinkedUnread        `json:"skipped"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Skipped) != 1 || out.Skipped[0].Kind != Radarr || out.Skipped[0].Address != InstanceAddress(other) {
		t.Fatalf("skipped %+v, want the one already read named so the page still counts it", out.Skipped)
	}
	if len(out.Linked) != 0 || len(out.Unread) != 1 || out.Unread[0].Kind != Radarr || out.Unread[0].Address != InstanceAddress(gone) {
		t.Fatalf("linked %+v unread %+v, want the skipped one left out and the gone one unread", out.Linked, out.Unread)
	}

	// The owner fixes the address once; the key comes from Seerr.
	from := `"linked_from":{"kind":"seerr","url":"` + su + `","address":"` + InstanceAddress(gone) + `"}`
	if rec := post("/api/v1/migrate/probe", `{"kind":"radarr","url":"`+ru+`","allow_local_network":true,`+from+`}`); rec.Code != http.StatusOK {
		t.Fatalf("probe: %d %s", rec.Code, rec.Body.String())
	}
	if rec := post("/api/v1/migrate/read", `{"kind":"radarr","url":"`+ru+`","allow_local_network":true,`+from+`}`); rec.Code != http.StatusCreated {
		t.Fatalf("read with linked_from: %d %s", rec.Code, rec.Body.String())
	}
	// An address Seerr does not keep gets no key.
	bad := `"linked_from":{"kind":"seerr","url":"` + su + `","address":"http://elsewhere:7878"}`
	if rec := post("/api/v1/migrate/read", `{"kind":"radarr","url":"`+ru+`","allow_local_network":true,`+bad+`}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown linked address: %d %s", rec.Code, rec.Body.String())
	}
	// Not the owner: refused.
	admin := webserver.ContextWith(ctx, webserver.Caller{UserID: 2, Name: "admin", Role: webserver.RoleAdmin, Via: "session"})
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/migrate/probe", strings.NewReader(`{"kind":"radarr","url":"`+ru+`",`+from+`}`)).WithContext(admin))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("admin: %d", rec.Code)
	}
	// The owner's API key: refused too, the saved key only goes out for a browser sign-in.
	byKey := webserver.ContextWith(ctx, webserver.Caller{UserID: 1, Name: "owner", Role: webserver.RoleOwner, Via: "api_key"})
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/migrate/read", strings.NewReader(`{"kind":"radarr","url":"`+ru+`","allow_local_network":true,`+from+`}`)).WithContext(byKey))
	if rec.Code != http.StatusForbidden || strings.Contains(rec.Body.String(), secretMark) {
		t.Fatalf("api key: %d %s", rec.Code, rec.Body.String())
	}
}

// One Radarr Seerr keeps under two addresses (same key) is read once, and a
// third address for it that does not answer needs no look.
func TestSeerrSameArrTwiceReadOnce(t *testing.T) {
	ctx := context.Background()
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	r := webserver.NewRouter()
	h := &Handlers{Store: st, Mover: func() *Mover { return nil }, SafeMode: func() bool { return false }}
	h.Routes(r)
	owner := webserver.ContextWith(ctx, webserver.Caller{UserID: 1, Name: "owner", Role: webserver.RoleOwner, Via: "session"})
	ru, rk := radarrAPIKeyed(t, "S3CR3T_RADARR_K", nil)
	alias := strings.Replace(ru, "127.0.0.1", "localhost", 1)
	su := seerrWithArrs(t, "S3CR3T_SEERR_K", []any{}, []any{arrEntry(ru, rk), arrEntry(alias, rk), arrEntry("http://127.0.0.1:1", rk)})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/migrate/read", strings.NewReader(`{"kind":"seerr","url":"`+su+`","key":"S3CR3T_SEERR_K","allow_local_network":true}`)).WithContext(owner))
	if rec.Code != http.StatusCreated {
		t.Fatalf("read: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Linked []struct{ Kind Kind } `json:"linked"`
		Unread []LinkedUnread        `json:"unread"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Linked) != 1 || len(out.Unread) != 0 {
		t.Fatalf("linked %+v unread %+v, want one Radarr and nothing to look at", out.Linked, out.Unread)
	}
}
