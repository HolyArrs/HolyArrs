package health

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// route: GET /api/v1/settings/ui
// route: PUT /api/v1/settings/ui
// The old trailer tool's update switch is gone from the answer. An older
// page that still sends it is answered as usual and the switch is ignored:
// nothing is stored for it and the other fields still save.
func TestUISettingsOldUpdateSwitchIgnored(t *testing.T) {
	t.Parallel()
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	m := http.NewServeMux()
	(&UIHandlers{Store: st, Actor: func(*http.Request) domain.Actor { return domain.System }}).Routes(m)
	srv := httptest.NewServer(m)
	t.Cleanup(srv.Close)
	do := func(t *testing.T, method, url, body string, want int) []byte {
		t.Helper()
		req, _ := http.NewRequest(method, url, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != want {
			t.Fatalf("%s %s: %d, want %d: %s", method, url, resp.StatusCode, want, b)
		}
		return b
	}
	path := srv.URL + "/api/v1/settings/ui"
	if b := string(do(t, "GET", path, "", 200)); strings.Contains(b, "keep_ytdlp_updated") {
		t.Fatalf("old switch answered %s", b)
	}
	for _, body := range []string{`{"keep_ytdlp_updated":false,"autoplay_trailers":false}`, `{"keep_ytdlp_updated":"yes"}`} {
		b := do(t, "PUT", path, body, 200)
		var got map[string]any
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatal(err)
		}
		if _, ok := got["keep_ytdlp_updated"]; ok {
			t.Fatalf("old switch answered after PUT %s", b)
		}
		if got["autoplay_trailers"] != false {
			t.Fatalf("other fields not saved beside it %s", b)
		}
	}
	var n int
	if err := st.Read().QueryRow("SELECT COUNT(*) FROM settings WHERE grp = 'this_app' AND key = 'ytdlp_auto_update'").Scan(&n); err != nil || n != 0 {
		t.Fatalf("old switch stored: %d %v", n, err)
	}
}
