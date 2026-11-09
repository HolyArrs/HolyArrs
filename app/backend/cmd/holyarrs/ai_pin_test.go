package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// rdpShape turns decoded JSON into its structure: keys sorted, each leaf
// replaced by its type name, a list by its first element.
func rdpShape(v any) any {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := map[string]any{}
		for _, k := range keys {
			out[k] = rdpShape(x[k])
		}
		return out
	case []any:
		if len(x) == 0 {
			return []any{}
		}
		return []any{rdpShape(x[0])}
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "bool"
	case nil:
		return "null"
	}
	return "other"
}

func rdpGoldenJSON(t *testing.T, name string, v any) {
	t.Helper()
	got, _ := json.MarshalIndent(v, "", "  ")
	path := filepath.Join("testdata", name+".golden.json")
	if os.Getenv("PIN_WRITE") == "1" {
		_ = os.MkdirAll("testdata", 0o755)
		if err := os.WriteFile(path, append(got, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != string(got)+"\n" {
		t.Fatalf("%s changed:\n got %s\nwant %s", name, got, want)
	}
}

// The tools an AI app sees (names, order, descriptions, input schemas) are
// frozen, for the owner and for a member.
func TestRDPAIToolListIsFrozen(t *testing.T) {
	f := newAIFix(t)
	f.turnOn(true)
	for who, key := range map[string]string{"owner": f.ownKey, "member": f.memKey} {
		_, v := f.rpc(key, "tools/list", "")
		res, _ := v["result"].(map[string]any)
		rdpGoldenJSON(t, "rdp_ai_tools_"+who, res)
	}
	_, v := f.rpc(f.ownKey, "initialize", `{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"pin","version":"1"}}`)
	res, _ := v["result"].(map[string]any)
	rdpGoldenJSON(t, "rdp_ai_initialize_shape", rdpShape(res))
}

// What each read tool answers keeps its shape.
func TestRDPAIToolAnswerShapes(t *testing.T) {
	f := newAIFix(t)
	f.turnOn(true)
	shapes := map[string]any{}
	for name, args := range map[string]string{
		"search_titles": `{"query":"dune"}`, "library_lookup": `{"name":"dune"}`, "calendar": `{}`, "downloads": `{}`, "my_asks": `{}`, "health": `{}`,
	} {
		m, msg := f.tool(f.ownKey, name, args)
		if msg != "" {
			shapes[name] = "error: " + msg
			continue
		}
		shapes[name] = rdpShape(m)
	}
	rdpGoldenJSON(t, "rdp_ai_tool_answers", shapes)
}

// The AI switch is one row in the settings table (group this_app, key
// mcp_enabled, on or off); the arr switches are rows of the same group.
func TestRDPSettingsRowsOfAIAndOldAppAPI(t *testing.T) {
	f := newAIFix(t)
	read := func(key string) (value, by string, n int) {
		_ = f.s.Read().QueryRow("SELECT count(*) FROM settings WHERE grp = 'this_app' AND key = ?", key).Scan(&n)
		_ = f.s.Read().QueryRow("SELECT value, updated_by FROM settings WHERE grp = 'this_app' AND key = ?", key).Scan(&value, &by)
		return
	}
	if _, _, n := read("mcp_enabled"); n != 0 {
		t.Fatal("a row before the first save")
	}
	// An empty body writes nothing.
	f.call(f.own, f.csrf, "", "PUT", "/api/v1/settings/ai", `{}`)
	if _, _, n := read("mcp_enabled"); n != 0 {
		t.Fatal("empty body wrote a row")
	}
	f.turnOn(true)
	if v, by, n := read("mcp_enabled"); v != "on" || n != 1 || by == "" {
		t.Fatalf("on: %q %q %d", v, by, n)
	}
	f.turnOn(false)
	if v, _, n := read("mcp_enabled"); v != "off" || n != 1 {
		t.Fatalf("off: %q %d", v, n)
	}
	// The arr API: each switch its own row; a missing field leaves its row alone.
	f.call(f.own, f.csrf, "", "PUT", "/api/v1/settings/old-app-api", `{"enabled":true}`)
	if v, _, n := read("old_app_api"); v != "on" || n != 1 {
		t.Fatalf("old_app_api: %q %d", v, n)
	}
	if _, _, n := read("old_app_api_delete_files"); n != 0 {
		t.Fatal("delete_files row written by an enabled-only save")
	}
	f.call(f.own, f.csrf, "", "PUT", "/api/v1/settings/old-app-api", `{"delete_files":true}`)
	if v, _, _ := read("old_app_api"); v != "on" {
		t.Fatalf("enabled changed by a delete_files-only save: %q", v)
	}
	if v, _, n := read("old_app_api_delete_files"); v != "on" || n != 1 {
		t.Fatalf("delete_files: %q %d", v, n)
	}
	_ = context.Background()
	_ = database.TimeLayout
}
