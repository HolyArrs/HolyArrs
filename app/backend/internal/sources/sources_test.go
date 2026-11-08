package sources

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

// route: GET /api/v1/sources
// route: POST /api/v1/sources
// route: PUT /api/v1/sources/{id}
// route: DELETE /api/v1/sources/{id}
// route: POST /api/v1/sources/{id}/test
// route: POST /api/v1/sources/{id}/pause
// route: POST /api/v1/sources/{id}/resume
// route: GET /api/v1/sources/{id}/stats
func TestSourcesAddTestPauseDelete(t *testing.T) {
	t.Parallel()
	srv, _ := sourceServer(t)
	key := "fake-indexer-key"
	idx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("apikey") != key {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(sourceCaps))
	}))
	t.Cleanup(idx.Close)

	var x SourceView
	raw, _ := json.Marshal(map[string]any{"name": "Lab", "format": "torznab", "url": idx.URL + "/api", "api_key": key, "allow_local_network": true, "priority": 10})
	body := string(raw)
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/sources", body, 201), &x)
	if x.APIKey != secrets.Masked || x.Health.State != "working" || x.Priority != 10 || !x.UseFor.AutoSearch {
		t.Fatalf("added %+v", x)
	}
	do(t, "POST", srv.URL+"/api/v1/sources", body, 409)
	do(t, "POST", srv.URL+"/api/v1/sources", `{"name":"Bad","format":"torznab","url":"ftp://x"}`, 400)
	id := fmt.Sprint(x.ID)

	// The list never holds the key.
	list := string(do(t, "GET", srv.URL+"/api/v1/sources", "", 200))
	if strings.Contains(list, key) || !strings.Contains(list, `"health"`) {
		t.Fatalf("list %s", list)
	}

	var res struct {
		OK     bool   `json:"ok"`
		Text   string `json:"text"`
		Health struct {
			State string `json:"state"`
		} `json:"health"`
	}
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/sources/"+id+"/test", "", 200), &res)
	if !res.OK || res.Health.State != "working" {
		t.Fatalf("test %+v", res)
	}

	// A wrong key: the test says so in plain words, and the key is not in it.
	wrong := strings.Join([]string{"not", "the", "key"}, "-")
	raw, _ = json.Marshal(map[string]any{"name": "Lab", "api_key": wrong})
	b := do(t, "PUT", srv.URL+"/api/v1/sources/"+id, string(raw), 200)
	if strings.Contains(string(b), wrong) {
		t.Fatalf("edit shows the key: %s", b)
	}
	b = do(t, "POST", srv.URL+"/api/v1/sources/"+id+"/test", "", 200)
	_ = json.Unmarshal(b, &res)
	if res.OK || !strings.Contains(res.Text, "turned down the key") || strings.Contains(string(b), wrong) {
		t.Fatalf("wrong key test %s", b)
	}
	// The masked placeholder keeps the stored key; a new host needs it again.
	do(t, "PUT", srv.URL+"/api/v1/sources/"+id, fmt.Sprintf(`{"name":"Lab","api_key":%q}`, secrets.Masked), 200)
	do(t, "PUT", srv.URL+"/api/v1/sources/"+id, `{"name":"Lab","url":"http://another.example/api"}`, 409)

	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/sources/"+id+"/pause", "", 200), &x)
	if x.Health.State != "disabled" {
		t.Fatalf("paused %+v", x.Health)
	}
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/sources/"+id+"/resume", "", 200), &x)
	if x.Health.State != "working" {
		t.Fatalf("resumed %+v", x.Health)
	}

	var st struct {
		Source SourceView `json:"source"`
		Days   []struct {
			Calls, Failures int
		} `json:"days"`
		Recent []struct {
			Op string `json:"op"`
			OK bool   `json:"ok"`
		} `json:"recent"`
	}
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/sources/"+id+"/stats?days=7", "", 200), &st)
	if len(st.Days) != 7 || st.Days[6].Calls != 2 || st.Days[6].Failures != 1 || len(st.Recent) != 2 || st.Source.Summary.LastSuccess == nil {
		t.Fatalf("stats %+v", st)
	}
	do(t, "GET", srv.URL+"/api/v1/sources/999/stats", "", 404)

	do(t, "DELETE", srv.URL+"/api/v1/sources/"+id, "", 204)
	do(t, "DELETE", srv.URL+"/api/v1/sources/"+id, "", 404)
	list = string(do(t, "GET", srv.URL+"/api/v1/sources", "", 200))
	if strings.Contains(list, `"Lab"`) {
		t.Fatalf("still listed %s", list)
	}
}

func TestSourceRSSKeepsPasskeyHidden(t *testing.T) {
	t.Parallel()
	srv, _ := sourceServer(t)
	b := string(do(t, "POST", srv.URL+"/api/v1/sources", `{"name":"Feed","format":"rss","url":"https://feed.example/rss?passkey=hidden-pk"}`, 201))
	if strings.Contains(b, "hidden-pk") || !strings.Contains(b, `"url":"https://feed.example/rss"`) || !strings.Contains(b, secrets.Masked) {
		t.Fatalf("rss %s", b)
	}
	do(t, "POST", srv.URL+"/api/v1/sources", `{"name":"Feed2","format":"rss","url":"https://feed.example/rss","use_for":{"auto_search":true}}`, 400)
}
