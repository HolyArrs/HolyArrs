package downloads

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Test in the Change form checks what is typed there, without saving it, the
// way the old apps test a form: a masked or empty key falls back to the saved
// one, and the saved Downloader, its key and its health stay as they were.
func TestDownloaderTestTypedValues(t *testing.T) {
	t.Parallel()
	key := "0123456789abcdef0123456789abcdef"
	other := "fedcba9876543210fedcba9876543210"
	wrongKey := strings.Repeat("x", 11)
	type result struct {
		OK     bool   `json:"ok"`
		Text   string `json:"text"`
		Health struct {
			State string `json:"state"`
		} `json:"health"`
	}
	for _, c := range []struct {
		name      string
		savedKey  string
		form      func(url string) map[string]any
		status    int
		ok        bool
		savedOKAt bool // a test without a body still passes after it
	}{
		{"typed wrong key fails, saved key kept", key, func(u string) map[string]any {
			return map[string]any{"name": "SAB", "category": "movies", "url": u, "secret": wrongKey}
		}, 200, false, true},
		{"typed right key passes over a saved wrong one", other, func(u string) map[string]any {
			return map[string]any{"name": "SAB", "category": "movies", "url": u, "secret": key}
		}, 200, true, false},
		{"masked key uses the saved one", key, func(u string) map[string]any {
			return map[string]any{"name": "SAB", "category": "movies", "url": u, "secret": secrets.Masked}
		}, 200, true, true},
		{"empty key uses the saved one", key, func(u string) map[string]any {
			return map[string]any{"name": "SAB", "category": "movies", "url": u, "secret": ""}
		}, 200, true, true},
		{"new address with the masked key asks for the key again", key, func(string) map[string]any {
			return map[string]any{"name": "SAB", "category": "movies", "url": "http://10.9.9.9:8080", "secret": secrets.Masked}
		}, 409, false, true},
		{"other port with the masked key asks for the key again", key, func(u string) map[string]any {
			return map[string]any{"name": "SAB", "category": "movies", "url": strings.Replace(u, "127.0.0.1:", "127.0.0.1:1", 1), "secret": secrets.Masked}
		}, 409, false, true},
		{"other path with the masked key asks for the key again", key, func(u string) map[string]any {
			return map[string]any{"name": "SAB", "category": "movies", "url": u + "/elsewhere", "secret": secrets.Masked}
		}, 409, false, true},
		{"other scheme with the masked key asks for the key again", key, func(u string) map[string]any {
			return map[string]any{"name": "SAB", "category": "movies", "url": strings.Replace(u, "http://", "https://", 1), "secret": secrets.Masked}
		}, 409, false, true},
		{"other user name with the masked key asks for the key again", key, func(u string) map[string]any {
			return map[string]any{"name": "SAB", "category": "movies", "url": u, "username": "someone", "secret": secrets.Masked}
		}, 409, false, true},
		{"same address with a trailing slash keeps the saved key", key, func(u string) map[string]any {
			return map[string]any{"name": "SAB", "category": "movies", "url": u + "/", "secret": secrets.Masked}
		}, 200, true, true},
		{"bad typed address is refused", key, func(string) map[string]any {
			return map[string]any{"name": "SAB", "category": "movies", "url": "http://user:pw@x", "secret": key}
		}, 400, false, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			srv, _ := dlServer(t)
			sab := fakeSAB(t, key)
			var d DownloaderView
			raw, _ := json.Marshal(map[string]any{"name": "SAB", "kind": "sabnzbd", "url": sab.URL, "secret": c.savedKey, "category": "movies", "allow_local_network": true})
			_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloaders", string(raw), 201), &d)
			id := itoa(d.ID)
			before := string(do(t, "GET", srv.URL+"/api/v1/downloaders", "", 200))

			var res result
			out := do(t, "POST", srv.URL+"/api/v1/downloaders/"+id+"/test", body(c.form(sab.URL)), c.status)
			if c.status == 200 {
				_ = json.Unmarshal(out, &res)
				if res.OK != c.ok {
					t.Fatalf("typed test %+v", res)
				}
			}
			if strings.Contains(string(out), key) || strings.Contains(string(out), other) {
				t.Fatalf("answer holds a key: %s", out)
			}
			// Nothing was saved: same Downloader, same key, same health.
			if after := string(do(t, "GET", srv.URL+"/api/v1/downloaders", "", 200)); after != before {
				t.Fatalf("typed test changed the saved Downloader:\n%s\n%s", before, after)
			}
			_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloaders/"+id+"/test", "", 200), &res)
			if res.OK != c.savedOKAt {
				t.Fatalf("saved test after typed test %+v", res)
			}
		})
	}
}

// A typed test can send the saved key, so like an edit it needs a strong
// caller; one let in by the local-network skip is refused, a session is not.
func TestDownloaderTestTypedNeedsStrongCaller(t *testing.T) {
	t.Parallel()
	key := "0123456789abcdef0123456789abcdef"
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	v, err := secrets.Open(context.Background(), s, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	m := http.NewServeMux()
	(&Web{Store: s, Metadata: metadata.Lab(), Secrets: v}).Routes(m)
	via := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if via != "" {
			r = r.WithContext(webserver.ContextWith(r.Context(), webserver.Caller{UserID: 1, Via: via}))
		}
		m.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	sab := fakeSAB(t, key)
	var d DownloaderView
	raw, _ := json.Marshal(map[string]any{"name": "SAB", "kind": "sabnzbd", "url": sab.URL, "secret": key, "category": "movies", "allow_local_network": true})
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloaders", string(raw), 201), &d)
	typed := body(map[string]any{"name": "SAB", "category": "movies", "url": sab.URL, "secret": secrets.Masked})
	for _, c := range []struct {
		via  string
		body string
		want int
	}{
		{"local_network", typed, 403},
		{"local_network", "", 200}, // the saved Downloader as it is
		{"session", typed, 200},
	} {
		via = c.via
		do(t, "POST", srv.URL+"/api/v1/downloaders/"+itoa(d.ID)+"/test", c.body, c.want)
	}
}
