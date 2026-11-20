package migrate

import (
	"strings"
	"testing"
)

// TestProwlarrSitesBecomeBuiltInSites: a Prowlarr site HolyArrs has built
// in is moved as a built-in site Source, read with no Prowlarr, and keeps the
// FlareSolverr it used there; other site definitions stay not moved.
func TestProwlarrSitesBecomeBuiltInSites(t *testing.T) {
	key := "S3CR3T_PROWLARR"
	def := func(id int, name, file string, tags ...int) map[string]any {
		return map[string]any{"id": id, "name": name, "implementation": "Cardigann", "protocol": "torrent", "enable": true, "priority": 20, "tags": tags,
			"fields": []any{map[string]any{"name": "definitionFile", "value": file}, map[string]any{"name": "baseUrl", "value": nil}}}
	}
	srv := fakeAPI(t, "X-Api-Key", key, map[string]any{
		"/api/v1/system/status": map[string]any{"appName": "Prowlarr", "version": "2.6.5"},
		"/api/v1/tag":           []any{map[string]any{"id": 1, "label": "flaresolverr"}},
		"/api/v1/indexer": []any{
			def(63, "1337x", "1337x", 1),
			def(12, "The Pirate Bay", "thepiratebay"),
			def(31, "OneJAV", "onejav"),
			map[string]any{"id": 58, "name": "Knaben", "implementation": "Knaben", "protocol": "torrent", "enable": false, "priority": 25,
				"fields": []any{map[string]any{"name": "baseUrl", "value": nil}}},
		},
		"/api/v1/indexerproxy": []any{map[string]any{"id": 1, "name": "FlareSolverr", "implementation": "FlareSolverr", "tags": []int{1},
			"fields": []any{map[string]any{"name": "host", "value": "http://flaresolverr:8191/"}, map[string]any{"name": "requestTimeout", "value": 60}}}},
	})
	s := read(t, api(Prowlarr, srv.URL, key))
	p := BuildPlan([]Input{{1, s}}, Choices{})

	x := item(t, p, GroupSources, "source:1337x")
	if sp := x.Source; sp.Format != "site" || sp.Site != "1337x" || sp.URL != "https://1337x.to" || sp.FlareSolverr != "http://flaresolverr:8191" || !sp.On || sp.Priority != 20 {
		t.Fatalf("1337x %+v", sp)
	}
	if !strings.Contains(x.Detail, "read directly with no Prowlarr") || !strings.Contains(x.Detail, "FlareSolverr at http://flaresolverr:8191") {
		t.Fatalf("1337x detail %q", x.Detail)
	}
	if sp := item(t, p, GroupSources, "source:the pirate bay").Source; sp.Site != "thepiratebay" || sp.URL != "https://apibay.org" || sp.FlareSolverr != "" {
		t.Fatalf("tpb %+v", sp)
	}
	if sp := item(t, p, GroupSources, "source:knaben").Source; sp.Site != "knaben" || sp.On {
		t.Fatalf("knaben %+v", sp)
	}
	if n, ok := notMoved(p, "prowlarr indexers[2]"); !ok || !strings.Contains(n.Why, "Prowlarr site definition") {
		t.Fatalf("onejav %+v", n)
	}
	// The FlareSolverr proxy went onto 1337x, so it is not listed as left behind.
	if _, ok := notMoved(p, "prowlarr indexer_proxies[0]"); ok {
		t.Fatal("FlareSolverr listed as not moved")
	}
	found := false
	for _, m := range p.Mapped {
		found = found || m == "prowlarr indexer_proxies[0]"
	}
	if !found {
		t.Fatalf("mapped %v", p.Mapped)
	}
}

// TestMoveAddsBuiltInSite: the move adds the built-in site Source, paused,
// with its FlareSolverr.
func TestMoveAddsBuiltInSite(t *testing.T) {
	l := newMoveLabWith(t, func(radarr, _, _ *Snapshot) {
		*radarr = Snapshot{Kind: Prowlarr, Version: "2.6.5",
			Indexers: []Provider{{Name: "1337x", Kind: "Cardigann", Enabled: true, Priority: 25, Protocol: "torrent", Tags: []string{"flaresolverr"},
				Settings: map[string]any{"definitionFile": "1337x"}}},
			IndexerProxies: []Provider{{Name: "FlareSolverr", Kind: "FlareSolverr", Tags: []string{"flaresolverr"}, Settings: map[string]any{"host": "http://flaresolverr:8191"}}}}
	})
	mv := l.move(t, Choices{})
	if got := lineOf(t, mv.Report, "source:1337x"); got.Result != ResCreated {
		t.Fatalf("source %+v", got)
	}
	if n := l.count(t, `SELECT count(*) FROM sources WHERE name = '1337x' AND format = 'site' AND json_extract(extra, '$.site') = '1337x'
		AND url = 'https://1337x.to' AND flaresolverr_url = 'http://flaresolverr:8191' AND state = 'disabled'`); n != 1 {
		t.Fatal("built-in site not added as planned")
	}
}
