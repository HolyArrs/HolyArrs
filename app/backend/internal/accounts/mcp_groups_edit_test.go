package accounts

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// The owner can change an AI key's tick boxes after it is made; the next
// call with the key sees the new ones. Scopes stay fixed.
func TestEditKeyMCPGroups(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	_, out := own.do(t, f, "POST", "/api/v1/auth/keys", `{"label":"ai","scopes":["mcp"]}`)
	var made struct {
		ID  int64  `json:"id"`
		Key string `json:"key"`
	}
	jsonOf(t, out, &made)
	path := "/api/v1/auth/keys/" + strconv.FormatInt(made.ID, 10)
	resp, out := own.do(t, f, "PUT", path, `{"mcp_groups":["approve","see"]}`)
	if resp.StatusCode != 200 || !strings.Contains(out, `"mcp_groups":["see","approve"]`) {
		t.Fatalf("edit: %d %s", resp.StatusCode, out)
	}
	who, ok := f.a.checkKey(context.Background(), made.Key)
	if !ok || !slices.Equal(who.MCPGroups, []string{"see", "approve"}) {
		t.Fatalf("next call sees %v", who.MCPGroups)
	}
	if _, out := own.do(t, f, "GET", "/api/v1/auth/keys", ""); !strings.Contains(out, `"mcp_groups":["see","approve"]`) {
		t.Fatalf("list: %s", out)
	}
	if resp, out := own.do(t, f, "PUT", path, `{"mcp_groups":["fly"]}`); resp.StatusCode != 400 {
		t.Fatalf("unknown group: %d %s", resp.StatusCode, out)
	}
	if resp, out := own.do(t, f, "PUT", path, `{"scopes":["all"]}`); resp.StatusCode != 400 {
		t.Fatalf("scopes edit: %d %s", resp.StatusCode, out)
	}
	if resp, out := own.do(t, f, "PUT", "/api/v1/auth/keys/999", `{"mcp_groups":["see"]}`); resp.StatusCode != 404 {
		t.Fatalf("missing key: %d %s", resp.StatusCode, out)
	}
	_, out = own.do(t, f, "POST", "/api/v1/auth/keys", `{"label":"plain","scopes":["read"]}`)
	var plain struct {
		ID int64 `json:"id"`
	}
	jsonOf(t, out, &plain)
	if resp, out := own.do(t, f, "PUT", "/api/v1/auth/keys/"+strconv.FormatInt(plain.ID, 10), `{"mcp_groups":["see"]}`); resp.StatusCode != 400 {
		t.Fatalf("groups on a read key: %d %s", resp.StatusCode, out)
	}
	// A member may not edit keys, and neither may a key.
	ana := join(t, f, invite(t, f, own, `{"role":"member"}`), "ana")
	if resp, _ := ana.do(t, f, "PUT", path, `{"mcp_groups":["admin"]}`); resp.StatusCode != 403 {
		t.Fatalf("member edit: %d", resp.StatusCode)
	}
	// An admin edits their own AI key, never the owner's.
	bo := join(t, f, invite(t, f, own, `{"role":"admin","perms":["settings"]}`), "bo")
	if resp, out := bo.do(t, f, "PUT", path, `{"mcp_groups":["admin","delete"]}`); resp.StatusCode != 404 {
		t.Fatalf("admin edits owner's key: %d %s", resp.StatusCode, out)
	}
	resp, out = bo.do(t, f, "POST", "/api/v1/auth/keys", `{"label":"bo ai","scopes":["mcp"]}`)
	var boKey struct {
		ID int64 `json:"id"`
	}
	jsonOf(t, out, &boKey)
	if resp, out := bo.do(t, f, "PUT", "/api/v1/auth/keys/"+strconv.FormatInt(boKey.ID, 10), `{"mcp_groups":["see","approve"]}`); resp.StatusCode != 200 {
		t.Fatalf("admin edits own key: %d %s", resp.StatusCode, out)
	}
	_, out = own.do(t, f, "POST", "/api/v1/auth/keys", `{"label":"all","scopes":["all"]}`)
	var allKey struct {
		Key string `json:"key"`
	}
	jsonOf(t, out, &allKey)
	resp, out = call(t, client(t), "PUT", f.srv.URL+path, `{"mcp_groups":["admin"]}`, map[string]string{APIKeyHeader: allKey.Key})
	if resp.StatusCode != 403 {
		t.Fatalf("key edit: %d %s", resp.StatusCode, out)
	}
}
