package migrate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Jackett answers its sign-in page without a key: a search names it on its
// port, and a page that is not Jackett is skipped.
func TestFindNamesJackett(t *testing.T) {
	jackett := page("", map[string]string{"/UI/Login": "<html><title>Jackett</title></html>"})
	other := page("", map[string]string{"/UI/Login": "<html>something else</html>"})
	defer jackett.Close()
	defer other.Close()
	old := findTargets
	t.Cleanup(func() { findTargets = old })
	findTargets = func([]string) []findTarget {
		jh, jp := hostPort(t, jackett.URL)
		oh, op := hostPort(t, other.URL)
		return []findTarget{{Host: jh, Port: jp, Kinds: []Kind{Jackett}}, {Host: oh, Port: op, Kinds: []Kind{Jackett}}}
	}
	got, _ := Find(context.Background(), nil, Jackett)
	if len(got) != 1 || got[0].Kind != Jackett || got[0].Address != jackett.URL {
		t.Fatalf("found %+v", got)
	}
	if ts := onlyKinds(old(nil), []Kind{Jackett}); len(ts) == 0 || ts[0].Port != 9117 {
		t.Fatalf("jackett targets %+v", ts)
	}
}

// Fetch key reads Jackett's settings call only when it answers without a
// sign-in; a locked Jackett or an odd key gives nothing.
func TestJackettKeyLookup(t *testing.T) {
	const key = "abcdefghij0123456789abcdefghij01"
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("a login was sent")
		}
		switch r.URL.Path {
		case "/open/api/v2.0/server/config":
			_, _ = w.Write([]byte(`{"api_key":"` + key + `","app_version":"0.22.0"}`))
		case "/locked/api/v2.0/server/config":
			http.Redirect(w, r, "/UI/Login", http.StatusFound)
		case "/odd/api/v2.0/server/config":
			_, _ = w.Write([]byte(`{"api_key":"NOT A KEY"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer app.Close()
	ctx := context.Background()
	if r := LookupKeyWith(ctx, Jackett, app.URL+"/open", true, KeyLogin{}); r.Key != key {
		t.Errorf("open: %+v", r)
	}
	for _, p := range []string{"/locked", "/odd"} {
		if r := LookupKeyWith(ctx, Jackett, app.URL+p, true, KeyLogin{}); r.Key != "" || r.NeedLogin {
			t.Errorf("%s: %+v", p, r)
		}
	}
}
