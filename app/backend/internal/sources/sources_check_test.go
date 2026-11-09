package sources

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// route: POST /api/v1/sources/check
func TestCheckAllSources(t *testing.T) {
	t.Parallel()
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	feed := `<?xml version="1.0"?><rss xmlns:torznab="http://torznab.com/schemas/2015/feed"><channel>
<item><title>The.Matrix.1999.1080p.BluRay-X</title><enclosure url="http://src/1.torrent" length="1000" type="application/x-bittorrent"/><torznab:attr name="seeders" value="5"/></item>
<item><title>The.Matrix.1999.720p.BluRay-X</title><enclosure url="http://src/2.torrent" length="900" type="application/x-bittorrent"/><torznab:attr name="seeders" value="5"/></item>
</channel></rss>`
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("t") == "caps" {
			_, _ = w.Write([]byte(sourceCaps))
			return
		}
		_, _ = w.Write([]byte(feed))
	}))
	t.Cleanup(good.Close)
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	t.Cleanup(bad.Close)
	add := func(name, url, state string) {
		_ = st.Write(context.Background(), func(tx *sql.Tx) error {
			_, err := database.Insert(tx, "sources", &domain.Source{Name: name, Protocol: "torrent", Format: "torznab", URL: url + "/api", State: state,
				AllowLocal: true, Kinds: []string{}, Categories: map[string][]int{}, UseFor: domain.SourceUse{AutoSearch: true, HandSearch: true}}, domain.System)
			if err != nil {
				t.Fatal(err)
			}
			return nil
		})
	}
	add("Good", good.URL, "enabled")
	add("Bad", bad.URL, "enabled")
	add("Sleepy", bad.URL, "paused")
	add("Off", bad.URL, "disabled")

	srv := routesServer(t, st, nil)

	var out struct {
		Query   string        `json:"query"`
		Sources []SourceCheck `json:"sources"`
	}
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/sources/check", `{"query":"The Matrix"}`, 200), &out)
	by := map[string]SourceCheck{}
	for _, c := range out.Sources {
		by[c.Name] = c
	}
	if len(out.Sources) != 3 || by["Off"].Name != "" {
		t.Fatalf("sources %+v", out.Sources)
	}
	if g := by["Good"]; !g.Answered || g.Count != 2 || g.Error != "" {
		t.Fatalf("good %+v", g)
	}
	if b := by["Bad"]; b.Answered || b.Error == "" {
		t.Fatalf("bad %+v", b)
	}
	if s := by["Sleepy"]; !s.Paused || s.Answered || s.MS != 0 {
		t.Fatalf("paused %+v", s)
	}
	// The failed Source is not paused by a test search, and calls are kept.
	var state string
	_ = st.Read().QueryRow("SELECT state FROM sources WHERE name = 'Bad'").Scan(&state)
	var calls int
	_ = st.Read().QueryRow("SELECT COUNT(*) FROM source_calls WHERE op = 'test'").Scan(&calls)
	if state != "enabled" || calls < 2 {
		t.Fatalf("state %s calls %d", state, calls)
	}
	// An empty body uses the default title.
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/sources/check", "", 200), &out)
	if !strings.Contains(out.Query, "Matrix") {
		t.Fatalf("query %q", out.Query)
	}
}
