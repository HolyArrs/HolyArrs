package migrate

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

const lookupKey = "0123456789abcdef0123456789ABCDEF"

func TestKeyLookup(t *testing.T) {
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok/initialize.json":
			_, _ = w.Write([]byte(`{"apiKey":"` + lookupKey + `","version":"6.0.0","instanceName":"Radarr"}`))
		case "/locked/initialize.json":
			w.WriteHeader(http.StatusUnauthorized)
		case "/html/initialize.json":
			_, _ = w.Write([]byte(`<html><body>sign in</body></html>`))
		case "/moved/initialize.json":
			http.Redirect(w, r, "/ok/initialize.json", http.StatusFound)
		case "/odd/initialize.json":
			_, _ = w.Write([]byte(`{"apiKey": "` + "not-a-key" + `"}`)) // not 32 hex characters
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(app.Close)

	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	r := webserver.NewRouter()
	h := &Handlers{Store: st, Mover: func() *Mover { return nil }, SafeMode: func() bool { return false }}
	h.Routes(r)
	owner := webserver.ContextWith(httptest.NewRequest("GET", "/", nil).Context(), webserver.Caller{UserID: 1, Name: "owner", Role: webserver.RoleOwner, Via: "session"})
	ask := func(kind, path string) (int, string) {
		body := `{"kind":"` + kind + `","url":"` + app.URL + path + `","allow_local_network":true}`
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/migrate/key-lookup", strings.NewReader(body)).WithContext(owner))
		var out struct {
			Key *string `json:"key"`
		}
		if rec.Code == http.StatusOK {
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Key == nil {
				t.Fatalf("%s %s: answer %s", kind, path, rec.Body)
			}
			return rec.Code, *out.Key
		}
		return rec.Code, ""
	}

	for _, k := range []string{"radarr", "sonarr", "lidarr", "prowlarr"} {
		if c, key := ask(k, "/ok"); c != 200 || key != lookupKey {
			t.Errorf("%s: %d, key found %v", k, c, key == lookupKey)
		}
	}
	for _, tc := range []struct{ kind, path string }{
		{"radarr", "/locked"}, {"radarr", "/html"}, {"radarr", "/moved"}, {"radarr", "/odd"}, {"radarr", "/missing"},
		{"bazarr", "/ok"}, {"qbittorrent", "/ok"}, {"seerr", "/ok"}, {"nope", "/ok"},
	} {
		if c, key := ask(tc.kind, tc.path); c != 200 || key != "" {
			t.Errorf("%s %s: %d, key given %v; want 200 and an empty key", tc.kind, tc.path, c, key != "")
		}
	}
	// Without the local-network leave the test server on this machine is not dialled.
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/migrate/key-lookup", strings.NewReader(`{"kind":"radarr","url":"`+app.URL+`/ok"}`)).WithContext(owner))
	if rec.Code != 200 || strings.Contains(rec.Body.String(), lookupKey) {
		t.Errorf("local address without the leave: %d %s", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/migrate/key-lookup", strings.NewReader(`not json`)).WithContext(owner))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad body: %d", rec.Code)
	}
}
