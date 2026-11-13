package sources

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
)

// listedHealth reads the Sources list, as a page reload does, and returns
// the health of the Source with that id.
func listedHealth(t *testing.T, base string, id int64) Health {
	t.Helper()
	var list struct {
		Sources []SourceView `json:"sources"`
	}
	_ = json.Unmarshal(do(t, "GET", base+"/api/v1/sources", "", 200), &list)
	for _, s := range list.Sources {
		if s.ID == id {
			return s.Health
		}
	}
	t.Fatalf("source %d not listed", id)
	return Health{}
}

// A failed test of an enabled Source shows as failed after a reload too,
// for a built-in site and an ordinary Source alike, and a later working
// test shows it working again.
// route: POST /api/v1/sources/{id}/test
func TestFailedTestKeepsShowingAfterReload(t *testing.T) {
	t.Parallel()
	login, err := os.ReadFile("testdata/sites/iptorrents_login.html")
	if err != nil {
		t.Fatal(err)
	}
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(login) }))
	t.Cleanup(site.Close)
	var up atomic.Bool
	idx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !up.Load() {
			http.Error(w, "down", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(sourceCaps))
	}))
	t.Cleanup(idx.Close)
	srv, _ := sourceServer(t)
	add := func(body string) SourceView {
		var x SourceView
		_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/sources", body, 201), &x)
		return x
	}
	test := func(id int64) (bool, Health) {
		var res struct {
			OK     bool   `json:"ok"`
			Health Health `json:"health"`
		}
		_ = json.Unmarshal(do(t, "POST", fmt.Sprintf("%s/api/v1/sources/%d/test", srv.URL, id), "", 200), &res)
		return res.OK, res.Health
	}

	cases := []struct {
		name, body, state string
	}{
		{"no cookie", `{"name":"IPT","format":"site","site":"iptorrents","url":"` + site.URL + `","allow_local_network":true}`, StateAuthFailed},
		{"junk cookie", `{"name":"IPT2","format":"site","site":"iptorrents","url":"` + site.URL + `","api_key":"uid=1; pass=x","allow_local_network":true}`, StateAuthFailed},
		{"ordinary", `{"name":"Lab","format":"torznab","url":"` + idx.URL + `/api","api_key":"k","allow_local_network":true}`, StateUnreachable},
	}
	ids := map[string]int64{}
	for _, c := range cases {
		x := add(c.body)
		ids[c.name] = x.ID
		ok, h := test(x.ID)
		if ok || h.State != c.state {
			t.Fatalf("%s: test ok=%v health %+v, want %s", c.name, ok, h, c.state)
		}
		if got := listedHealth(t, srv.URL, x.ID); got.State != c.state || got.Text != h.Text {
			t.Fatalf("%s: after reload %+v, right after the test %+v", c.name, got, h)
		}
	}

	up.Store(true)
	if ok, h := test(ids["ordinary"]); !ok || h.State != StateWorking {
		t.Fatalf("working test %v %+v", ok, h)
	}
	if got := listedHealth(t, srv.URL, ids["ordinary"]); got.State != StateWorking {
		t.Fatalf("after a working test %+v", got)
	}
}
