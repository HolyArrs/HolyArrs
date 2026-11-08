package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/ai"
)

// The per-hour action limit is applied by tools/call: every call that carries
// a confirm token counts, and the one over the limit gets a 429 with
// Retry-After and JSON-RPC error -32000. Calls without a token do not count.
func TestMCPActionLimitThroughRPC(t *testing.T) {
	f := newAIFix(t)
	f.turnOn(true)
	for i := 0; i < ai.ActsPerHour; i++ {
		first, msg := f.tool(f.memKey, "ask_for_title", fmt.Sprintf(`{"tmdb_id":"%d"}`, 100+i))
		tok, _ := first["confirm_token"].(string)
		if tok == "" {
			t.Fatalf("no token: %s", msg)
		}
		f.tool(f.memKey, "ask_for_title", fmt.Sprintf(`{"tmdb_id":"%d","confirm_token":%q}`, 100+i, tok))
	}
	// A first call (no token) is still allowed.
	f.tool(f.memKey, "ask_for_title", `{"tmdb_id":"200"}`)
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ask_for_title","arguments":{"tmdb_id":"200","confirm_token":"x"}}}`
	req, _ := http.NewRequest("POST", f.srv.URL+"/api/v1/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(accounts.APIKeyHeader, f.memKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	st, raw, retry := resp.StatusCode, string(b), resp.Header.Get("Retry-After")
	if st != http.StatusTooManyRequests || retry == "" {
		t.Fatalf("status %d retry %q %s", st, retry, raw)
	}
	var v map[string]any
	_ = json.Unmarshal([]byte(raw), &v)
	e, _ := v["error"].(map[string]any)
	if e["code"] != float64(-32000) {
		t.Fatalf("error %v", v)
	}
	// Another key is not affected.
	if st, _ := f.rpc(f.ownKey, "tools/call", `{"name":"calendar","arguments":{}}`); st != 200 {
		t.Fatalf("owner key status %d", st)
	}
}
