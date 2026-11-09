package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
)

// A Kid's AI key runs through the same Kid limits as the Kid's own
// session: Library lookups and search never name a title above the limit,
// and asking for one is refused.
func TestMCPKidKeyKeepsKidLimits(t *testing.T) {
	k := newKidRig(t)
	k.owner.call("PUT", "/api/v1/settings/ai", `{"enabled":true}`, 200)
	// Making keys needs the settings permission.
	if err := k.st.WriteCore(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE users SET perms = '["ask","settings"]' WHERE id = ?`, k.kai.id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var made struct {
		Key string `json:"key"`
	}
	_ = json.Unmarshal(k.kai.call("POST", "/api/v1/auth/keys", `{"label":"claude","scopes":["mcp"]}`, 201), &made)
	if made.Key == "" {
		t.Fatal("no key")
	}
	rpc := func(name, args string) (string, bool) {
		t.Helper()
		body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + name + `","arguments":` + args + `}}`
		req, _ := http.NewRequest("POST", k.srv.URL+"/api/v1/mcp", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(accounts.APIKeyHeader, made.Key)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != 200 {
			t.Fatalf("%s: %d %s", name, resp.StatusCode, b)
		}
		var v struct {
			Result struct {
				IsError bool `json:"isError"`
			} `json:"result"`
		}
		_ = json.Unmarshal(b, &v)
		return string(b), v.Result.IsError
	}
	for _, q := range []string{"Iron", "Night", "Cold"} {
		b, _ := rpc("library_lookup", fmt.Sprintf(`{"name":%q}`, q))
		if n := mentionsBlocked(b); n != "" {
			t.Fatalf("library_lookup %s showed %s: %s", q, n, b)
		}
		b, _ = rpc("search_titles", fmt.Sprintf(`{"query":%q}`, q))
		if n := mentionsBlocked(b); n != "" {
			t.Fatalf("search_titles %s showed %s: %s", q, n, b)
		}
	}
	// The Kid's own limit still lets an allowed title through the lookup.
	if b, isErr := rpc("library_lookup", `{"name":"Paper"}`); isErr || !strings.Contains(b, "Paper Kite") {
		t.Fatalf("allowed title missing: %s", b)
	}
	// Asking for an R movie is refused at either step.
	b, isErr := rpc("ask_for_title", `{"tmdb_id":"4204"}`)
	if !isErr {
		var v struct {
			Result struct {
				Structured map[string]any `json:"structuredContent"`
			} `json:"result"`
		}
		_ = json.Unmarshal([]byte(b), &v)
		tok, _ := v.Result.Structured["confirm_token"].(string)
		if b, isErr = rpc("ask_for_title", `{"tmdb_id":"4204","confirm_token":"`+tok+`"}`); !isErr {
			t.Fatalf("kid ask went through: %s", b)
		}
	}
	var n int
	if err := k.st.Read().QueryRow("SELECT count(*) FROM ask_people WHERE user_id = ?", k.kai.id).Scan(&n); err != nil || n != 0 {
		t.Fatalf("asks %d %v", n, err)
	}
}
