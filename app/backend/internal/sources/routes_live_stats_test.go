package sources

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

type recPub struct {
	mu  sync.Mutex
	got []map[string]any
	top []string
}

func (p *recPub) Publish(topic string, v any) uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.top = append(p.top, topic)
	m, _ := v.(map[string]any)
	p.got = append(p.got, m)
	return uint64(len(p.top))
}

// sourceRig serves the routes with a Searcher and Publisher the test can see.
func sourceRig(t *testing.T) (*httptest.Server, *Searcher, *recPub, string) {
	t.Helper()
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	v, err := secrets.Open(t.Context(), st, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	sr := &Searcher{Store: st, Secrets: v}
	pub := &recPub{}
	h := &Handlers{Store: st, Vault: v, Live: pub, Search: func() *Searcher { return sr }, Catalog: rigCatalog{}, OldApps: rigOldApps{},
		PartOn: func(parts.Part) bool { return true }}
	rt := webserver.NewRouter()
	h.Routes(rt)
	srv := httptest.NewServer(rt)
	t.Cleanup(srv.Close)
	idx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(sourceCaps)) }))
	t.Cleanup(idx.Close)
	raw, _ := json.Marshal(map[string]any{"name": "Lab", "format": "torznab", "url": idx.URL + "/api", "api_key": "k", "allow_local_network": true})
	return srv, sr, pub, string(raw)
}

func TestSourceRoutesPublishLiveHealth(t *testing.T) {
	t.Parallel()
	srv, _, pub, body := sourceRig(t)
	var x SourceView
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/sources", body, 201), &x)
	do(t, "POST", fmt.Sprintf("%s/api/v1/sources/%d/pause", srv.URL, x.ID), "", 200)
	pub.mu.Lock()
	defer pub.mu.Unlock()
	if len(pub.top) != 2 {
		t.Fatalf("events %v", pub.top)
	}
	for i, tp := range pub.top {
		if tp != "source" || pub.got[i]["id"] != x.ID || pub.got[i]["health"] == nil {
			t.Fatalf("event %d: %s %v", i, tp, pub.got[i])
		}
	}
	if h, _ := pub.got[1]["health"].(Health); h.State != StateDisabled {
		t.Fatalf("pause health %+v", pub.got[1]["health"])
	}
}

func TestSourceStatsClampsDays(t *testing.T) {
	t.Parallel()
	srv, _, _, body := sourceRig(t)
	var x SourceView
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/sources", body, 201), &x)
	for _, q := range []string{"?days=500", "?days=0", "?days=x", ""} {
		var out map[string]any
		_ = json.Unmarshal(do(t, "GET", fmt.Sprintf("%s/api/v1/sources/%d/stats%s", srv.URL, x.ID, q), "", 200), &out)
		if days, _ := out["days"].([]any); len(days) != 14 {
			t.Fatalf("%s: days %d", q, len(days))
		}
		for _, k := range []string{"source", "recent"} {
			if _, ok := out[k]; !ok {
				t.Fatalf("%s: no %s in %v", q, k, out)
			}
		}
	}
	var out map[string]any
	_ = json.Unmarshal(do(t, "GET", fmt.Sprintf("%s/api/v1/sources/%d/stats?days=90", srv.URL, x.ID), "", 200), &out)
	if days, _ := out["days"].([]any); len(days) != 90 {
		t.Fatalf("90 days: %d", len(days))
	}
}

func TestDeleteSourceForgetsSearcherState(t *testing.T) {
	t.Parallel()
	srv, sr, _, body := sourceRig(t)
	var x SourceView
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/sources", body, 201), &x)
	sr.quietMu.Lock()
	if sr.quietCalls == nil {
		sr.quietCalls = map[int64]*quietSourceCalls{}
	}
	sr.quietCalls[x.ID] = &quietSourceCalls{days: map[string]quietDay{}}
	sr.quietMu.Unlock()
	sr.mu.Lock()
	if sr.clients == nil {
		sr.clients = map[int64]*Client{}
	}
	sr.clients[x.ID] = &Client{}
	sr.mu.Unlock()

	do(t, "DELETE", fmt.Sprintf("%s/api/v1/sources/%d", srv.URL, x.ID), "", 204)
	sr.quietMu.Lock()
	_, q := sr.quietCalls[x.ID]
	sr.quietMu.Unlock()
	sr.mu.Lock()
	_, c := sr.clients[x.ID]
	sr.mu.Unlock()
	if q || c {
		t.Fatalf("state kept: quiet %v client %v", q, c)
	}
}
