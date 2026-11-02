package api

import (
	_ "embed"
	"encoding/json"
	"strconv"
	"strings"
)

// edgeRuleJSON is the committed Cloudflare cache rule (zone coderluii.dev,
// phase http_request_cache_settings). Its paths must equal EdgePrefixes();
// TestEdgeRuleMatchesPolicies fails on any difference, and
// `metadata edge-rule check` fails when the live rule differs from it.
//
//go:embed edgerule.json
var edgeRuleJSON []byte

// EdgeRule is the committed cache rule in the shape Cloudflare stores it.
type EdgeRule struct {
	Description      string          `json:"description"`
	Enabled          bool            `json:"enabled"`
	Action           string          `json:"action"`
	ActionParameters json.RawMessage `json:"action_parameters"`
	Host             string          `json:"host"`
	Paths            []string        `json:"paths"`
}

// CommittedEdgeRule parses the committed rule.
func CommittedEdgeRule() (EdgeRule, error) {
	var r EdgeRule
	err := json.Unmarshal(edgeRuleJSON, &r)
	return r, err
}

// Expression is the Cloudflare filter expression of the rule.
func (r EdgeRule) Expression() string {
	parts := make([]string, len(r.Paths))
	for i, p := range r.Paths {
		parts[i] = "starts_with(http.request.uri.path, " + strconv.Quote(p) + ")"
	}
	return "(http.host eq " + strconv.Quote(r.Host) + " and (" + strings.Join(parts, " or ") + "))"
}
