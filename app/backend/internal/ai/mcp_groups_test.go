package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/ai/mcpkeys"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// without is every group but g.
func without(g mcpkeys.Group) []string {
	var out []string
	for _, x := range mcpkeys.All {
		if x != g {
			out = append(out, string(x))
		}
	}
	return out
}

// A key missing one tick box cannot reach a route of that group, and the
// error names the tick box; with it ticked the route runs.
func TestCallRefusedPerGroup(t *testing.T) {
	routes := map[mcpkeys.Group][2]string{
		mcpkeys.See:      {"GET", "/api/v1/calendar?from=x"},
		mcpkeys.Add:      {"POST", "/api/v1/asks"},
		mcpkeys.Control:  {"POST", "/api/v1/search/run"},
		mcpkeys.Settings: {"PUT", "/api/v1/settings/naming"},
		mcpkeys.Delete:   {"DELETE", "/api/v1/asks/3"},
		mcpkeys.Admin:    {"POST", "/api/v1/backups"},
		mcpkeys.Approve:  {"POST", "/api/v1/asks/3/approve"},
		mcpkeys.Plugins:  {"GET", "/api/v1/plugin-pages"},
	}
	for _, g := range mcpkeys.All {
		rt, ok := routes[g]
		if !ok {
			t.Fatalf("no route for %s", g)
		}
		if got, _ := mcpkeys.GroupFor(rt[0], strings.SplitN(rt[1], "?", 2)[0]); got != g {
			t.Fatalf("%s %s is %s, want %s", rt[0], rt[1], got, g)
		}
		gr := newRig(t)
		gr.who.MCPGroups = without(g)
		if g == mcpkeys.Approve {
			gr.who.MCPGroups = slices.DeleteFunc(gr.who.MCPGroups, func(s string) bool { return s == string(mcpkeys.Admin) })
		}
		code, v := gr.h.call(gr.request(), gr.who, webserver.ScopeWrite, rt[0], rt[1], nil)
		want := "This key can't " + g.Label() + ". The owner can turn it on in Settings → Keys."
		if code != http.StatusForbidden || routeError(code, v) != want {
			t.Errorf("%s: %d %v", g, code, v)
		}
		gr.who.MCPGroups = []string{string(g)}
		if code, _ := gr.h.call(gr.request(), gr.who, webserver.ScopeWrite, rt[0], rt[1], nil); code == http.StatusForbidden {
			t.Errorf("%s ticked: still refused", g)
		}
	}
}

// The real tools stop at the tick box: search needs See, asking needs Add.
func TestToolsRefusedByGroups(t *testing.T) {
	g := newRig(t)
	g.who.MCPGroups = without(mcpkeys.See)
	if _, text, isErr := g.tool("search_titles", map[string]any{"query": "space"}); !isErr || !strings.Contains(text, "can't See") {
		t.Errorf("search without See: %v %q", isErr, text)
	}
	g.who.MCPGroups = without(mcpkeys.Add)
	m, _, isErr := g.tool("ask_for_title", map[string]any{"tmdb_id": "1", "kind": "movie"})
	if isErr {
		t.Fatal("first step failed")
	}
	tok, _ := m["confirm_token"].(string)
	_, text, isErr := g.tool("ask_for_title", map[string]any{"tmdb_id": "1", "kind": "movie", "confirm_token": tok})
	if !isErr || !strings.Contains(text, "This key can't Add.") || g.asked != 0 {
		t.Errorf("ask without Add: %v %q asked=%d", isErr, text, g.asked)
	}
	g.who.MCPGroups = nil
	if _, _, isErr := g.tool("downloads", nil); !isErr {
		t.Error("a key with no tick boxes read downloads")
	}
}

// Approving or declining a request takes the Approve box or Admin.
func TestApproveTakesApproveOrAdmin(t *testing.T) {
	for _, p := range []string{"/api/v1/asks/{id}/approve", "/api/v1/asks/{id}/turn-down"} {
		for _, c := range []struct {
			set mcpkeys.Set
			ok  bool
		}{{mcpkeys.Set{mcpkeys.Approve}, true}, {mcpkeys.Set{mcpkeys.Admin}, true}, {mcpkeys.Default, false}, {mcpkeys.Set{mcpkeys.Delete}, false}} {
			if got := mcpkeys.Allows(c.set, "POST", p); got != c.ok {
				t.Errorf("%s with %v: %v, want %v", p, c.set, got, c.ok)
			}
		}
	}
}

func TestDefaultGroups(t *testing.T) {
	want := []mcpkeys.Group{mcpkeys.See, mcpkeys.Add, mcpkeys.Control, mcpkeys.Settings}
	if !slices.Equal(mcpkeys.Default, want) {
		t.Fatalf("default %v", mcpkeys.Default)
	}
}

// With AI connections off, /mcp answers 404, as before.
func TestMCPSwitchOff(t *testing.T) {
	g := newRig(t)
	if err := saveEnabled(context.Background(), g.st, database.System, false); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"/mcp", "/api/v1/mcp"} {
		r := httptest.NewRequest("POST", p, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
		r = r.WithContext(webserver.ContextWith(r.Context(), g.who))
		rec := httptest.NewRecorder()
		g.h.serveRPC(rec, r)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: %d %s", p, rec.Code, rec.Body.String())
		}
	}
}

func TestPluginToolsNeedTheirBox(t *testing.T) {
	h := &Handlers{PluginTools: fakePluginTools{}}
	for _, c := range []struct {
		groups []string
		on     bool
	}{
		{[]string{"see", "add", "control", "settings"}, false},
		{[]string{"admin"}, false},
		{[]string{"see", "plugins"}, true},
	} {
		if got := h.pluginToolsOn(webserver.Caller{MCPGroups: c.groups}); got != c.on {
			t.Errorf("%v: %v", c.groups, got)
		}
	}
	if (&Handlers{}).pluginToolsOn(webserver.Caller{MCPGroups: []string{"plugins"}}) {
		t.Error("on without plugin tools")
	}
	res, _ := (&Handlers{}).callTool(httptest.NewRequest("POST", "/mcp", nil), webserver.Caller{}, "plugin.x.y", nil)
	if res["isError"] != true {
		t.Error("plugin tool ran with plugin tools off")
	}
}

type fakePluginTools struct{}

func (fakePluginTools) List(webserver.Caller) []map[string]any { return nil }
func (fakePluginTools) Call(*http.Request, webserver.Caller, string, map[string]any) (any, error) {
	return nil, nil
}
