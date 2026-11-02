package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
)

// One request answers a whole row's picture records. Missing ones are
// queued only for an install, only at prefetch priority under its share,
// at most 20 per ask, each id once; the answer is then kept a minute only.
func TestMusicPicturesBatch(t *testing.T) {
	const a, b = "a74b1b7f-71a5-4011-9441-d0b5e4122711", "b1392450-e666-3926-a536-22c65f834433"
	var row []string
	urgent := 0
	s, _, calls := setup(Options{Burst: 1000,
		Images: &Images{Want: func(k, m string) { urgent++ }, WantAhead: func(k, m string) { urgent++ },
			WantNow: func(context.Context, string, string) { urgent++ },
			WantRow: func(ctx context.Context, k, m string) {
				if ctx == nil {
					t.Error("no install context")
				}
				row = append(row, k+":"+m)
			}},
		Pictures: func(_ context.Context, kind, mbid string) map[string]imgstore.Info {
			if kind == "artist" && mbid == a {
				return map[string]imgstore.Info{"photo": {Version: "v1", Width: 500, Height: 500, Color: "#112233"}}
			}
			return map[string]imgstore.Info{}
		}})
	w := req(s, "/v1/music/pictures?artists="+a+"&albums="+b+","+b+","+strings.ToUpper(b), "synthetic-test-token")
	var got map[string]map[string]map[string]imgstore.Info
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &got) != nil || got["artists"][a]["photo"].Color != "#112233" || len(got["albums"]) != 1 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if len(row) != 1 || row[0] != "album:"+b || urgent != 0 {
		t.Fatalf("a duplicate is queued once, at prefetch priority: %v urgent %d", row, urgent)
	}
	if w.Header().Get(partialHeader) == "" && w.Header().Get("Cache-Control") == "" {
		t.Fatalf("missing pictures make the answer short-lived: %v", w.Header())
	}
	// Anonymous: the records only, nothing queued.
	row = nil
	if w := req(s, "/v1/music/pictures?albums="+b, ""); w.Code != 200 || len(row) != 0 || w.Header().Get("Cache-Control") != partialPolicy {
		t.Fatalf("anonymous queued %v (%d %v)", row, w.Code, w.Header())
	}
	// At most 20 queued per ask.
	var many []string
	for i := range 30 {
		many = append(many, fmt.Sprintf("%08d-e666-3926-a536-22c65f834433", i))
	}
	if w := req(s, "/v1/music/pictures?albums="+strings.Join(many, ","), "synthetic-test-token"); w.Code != 200 || len(row) != 20 {
		t.Fatalf("queued %d of 30, want 20", len(row))
	}
	for _, bad := range []string{"/v1/music/pictures", "/v1/music/pictures?songs=" + a, "/v1/music/pictures?artists=nope", "/v1/music/pictures?artists=" + strings.Repeat(a+",", 100) + a} {
		if w := req(s, bad, ""); w.Code != 400 {
			t.Fatalf("%s: %d", bad, w.Code)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("reached a provider")
	}
}
