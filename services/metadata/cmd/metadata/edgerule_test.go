package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CoderLuii/holyarrs-metadata/internal/api"
)

// fakeRuleset serves a cache-settings entrypoint holding one other rule and
// (optionally) the HolyArrs rule with the given expression.
func fakeRuleset(t *testing.T, expr string) (*httptest.Server, *string, *int) {
	live, writes := expr, 0
	cf := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer synthetic-cf-token" {
			w.WriteHeader(403)
			return
		}
		rule, _ := api.CommittedEdgeRule()
		switch {
		case r.Method == "GET" && r.URL.Path == "/zones":
			_, _ = w.Write([]byte(`{"success":true,"result":[{"id":"zone1"}]}`))
		case r.Method == "GET" && r.URL.Path == "/zones/zone1/rulesets/phases/http_request_cache_settings/entrypoint":
			rules := []map[string]any{{"id": "other", "description": "someone else", "expression": "true", "action": "set_cache_settings"}}
			if live != "" {
				rules = append(rules, map[string]any{"id": "r1", "description": rule.Description, "enabled": true,
					"action": rule.Action, "action_parameters": rule.ActionParameters, "expression": live})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "result": map[string]any{"id": "rs1", "rules": rules}})
		case (r.Method == "PATCH" && r.URL.Path == "/zones/zone1/rulesets/rs1/rules/r1") ||
			(r.Method == "POST" && r.URL.Path == "/zones/zone1/rulesets/rs1/rules"):
			var b struct{ Expression string }
			_ = json.NewDecoder(r.Body).Decode(&b)
			live = b.Expression
			writes++
			_, _ = w.Write([]byte(`{"success":true}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(cf.Close)
	return cf, &live, &writes
}

func TestEdgeRuleCheckFailsOnDriftAndApplyFixes(t *testing.T) {
	t.Setenv("CLOUDFLARE_API_TOKEN", "synthetic-cf-token")
	rule, _ := api.CommittedEdgeRule()
	cf, live, writes := fakeRuleset(t, strings.Replace(rule.Expression(), ` or starts_with(http.request.uri.path, "/v1/img/")`, "", 1))
	err := run([]string{"edge-rule", "check", "-api", cf.URL})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "drift") || !strings.Contains(err.Error(), "/v1/img/") {
		t.Fatalf("drift not reported: %v", err)
	}
	if err := run([]string{"edge-rule", "apply", "-api", cf.URL}); err != nil {
		t.Fatal(err)
	}
	if *live != rule.Expression() || *writes != 1 {
		t.Fatalf("apply wrote %d times: %s", *writes, *live)
	}
	if err := run([]string{"edge-rule", "check", "-api", cf.URL}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"edge-rule", "apply", "-api", cf.URL}); err != nil || *writes != 1 {
		t.Fatalf("apply without drift wrote again: %v %d", err, *writes)
	}
}

func TestEdgeRuleMissingRuleIsDriftAndApplyAdds(t *testing.T) {
	t.Setenv("CLOUDFLARE_API_TOKEN", "synthetic-cf-token")
	cf, live, _ := fakeRuleset(t, "")
	if err := run([]string{"edge-rule", "check", "-api", cf.URL}); err == nil {
		t.Fatal("missing rule not reported")
	}
	if err := run([]string{"edge-rule", "apply", "-api", cf.URL}); err != nil || *live == "" {
		t.Fatalf("apply did not add: %v", err)
	}
	t.Setenv("CLOUDFLARE_API_TOKEN", "")
	if err := run([]string{"edge-rule", "check", "-api", cf.URL}); err == nil {
		t.Fatal("no token accepted")
	}
}
