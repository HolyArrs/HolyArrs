package accounts

import (
	"strings"
	"testing"
)

// An AI key's tick boxes come back on create and list; other keys take
// none. Like scopes, they are fixed when the key is made.
func TestKeyMCPGroups(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	resp, out := own.do(t, f, "POST", "/api/v1/auth/keys", `{"label":"ai","scopes":["mcp"]}`)
	if resp.StatusCode != 201 || !strings.Contains(out, `"mcp_groups":["see","add","control","settings"]`) {
		t.Fatalf("default groups: %d %s", resp.StatusCode, out)
	}
	if resp, out := own.do(t, f, "POST", "/api/v1/auth/keys", `{"label":"ai2","scopes":["mcp"],"mcp_groups":["delete","see"]}`); resp.StatusCode != 201 || !strings.Contains(out, `"mcp_groups":["see","delete"]`) {
		t.Fatalf("chosen groups: %d %s", resp.StatusCode, out)
	}
	if resp, out := own.do(t, f, "POST", "/api/v1/auth/keys", `{"label":"x","scopes":["mcp"],"mcp_groups":["fly"]}`); resp.StatusCode != 400 {
		t.Fatalf("unknown group: %d %s", resp.StatusCode, out)
	}
	if resp, out := own.do(t, f, "POST", "/api/v1/auth/keys", `{"label":"x","scopes":["read"],"mcp_groups":["see"]}`); resp.StatusCode != 400 {
		t.Fatalf("groups on a read key: %d %s", resp.StatusCode, out)
	}
	if _, out := own.do(t, f, "GET", "/api/v1/auth/keys", ""); !strings.Contains(out, `"mcp_groups":["see","delete"]`) || strings.Count(out, "mcp_groups") != 2 {
		t.Fatalf("list: %s", out)
	}
}
