package api

import (
	"encoding/json"
	"strings"
	"testing"
)

// route: GET /api/v1/settings/ui
// route: PUT /api/v1/settings/ui
// The old trailer tool's update switch is gone from the answer. An older
// page that still sends it is answered as usual and the switch is ignored:
// nothing is stored for it and the other fields still save.
func TestUISettingsOldUpdateSwitchIgnored(t *testing.T) {
	t.Parallel()
	srv, st := discoverRig(t, trailerTMDB(t))
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
