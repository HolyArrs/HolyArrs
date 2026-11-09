package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// routePackages are the packages that register /api/v1 routes on the
// server's mux. Every such route must be in openapi.json; that each one is
// served to at least one test request is checked by each package's
// TestMain (internal/routehits), not by comments.
var routePackages = []string{".", "../../internal/accounts", "../../internal/backups", "../../internal/alerts", "../../internal/health", "../../internal/updates", "../../internal/mediaservers", "../../internal/metadata", "../../internal/sources", "../../internal/subtitles", "../../internal/ai", "../../internal/arrapi", "../../internal/migrate", "../../internal/movies", "../../internal/shows", "../../internal/library", "../../internal/mediafiles", "../../internal/mediafiles/datapack", "../../internal/importer", "../../internal/setup", "../../internal/downloads", "../../internal/requests", "../../internal/music", "../../internal/discover", "../../internal/core/webserver"}

type openapiOp struct {
	Summary    string         `json:"summary"`
	Perm       string         `json:"x-holyarrs-permission"`
	Responses  map[string]any `json:"responses"`
	Parameters []struct {
		Name string `json:"name"`
		In   string `json:"in"`
	} `json:"parameters"`
}

var updateSpec = flag.Bool("update", false, "rewrite openapi.json from the code (go generate ./cmd/holyarrs)")

// generateSpec builds openapi.json from the code: the operations are
// exactly the registered /api/v1 routes, and each one's permission
// (accounts.RoutePerms), API key scope (accounts.RouteScope), part (parts.Routes)
// and path parameters come from the route table. The written parts
// (summaries, descriptions, bodies, answers) are kept from the current
// file; a new route gets an empty entry that fails the check below until
// it is described.
func generateSpec(t *testing.T) []byte {
	t.Helper()
	openapiJSON := readSpec(t)
	dec := json.NewDecoder(bytes.NewReader(openapiJSON))
	dec.UseNumber()
	var doc map[string]any
	if err := dec.Decode(&doc); err != nil {
		t.Fatalf("openapi.json: %v", err)
	}
	var registered []string
	for _, dir := range routePackages {
		rs, err := webserver.RegisteredRoutes(dir)
		if err != nil {
			t.Fatal(err)
		}
		registered = append(registered, rs...)
	}
	if len(registered) < 150 {
		t.Fatalf("found only %d routes; the pattern no longer matches the code", len(registered))
	}
	old, _ := doc["paths"].(map[string]any)
	paths := map[string]any{}
	param := regexp.MustCompile(`\{(\w+)\}`)
	for _, r := range registered {
		method, path, _ := strings.Cut(r, " ")
		m := strings.ToLower(method)
		var op map[string]any
		if ops, ok := old[path].(map[string]any); ok {
			op, _ = ops[m].(map[string]any)
		}
		if op == nil {
			op = map[string]any{"summary": "", "responses": map[string]any{}}
		}
		perm := string(accounts.RoutePerms[r])
		if perm == "" {
			perm = "owner"
		}
		op["x-holyarrs-permission"] = perm
		op["x-holyarrs-scope"] = accounts.RouteScope(r)
		op["x-holyarrs-part"] = string(parts.Routes()[r])
		params, _ := op["parameters"].([]any)
		for _, p := range param.FindAllStringSubmatch(path, -1) {
			found := false
			for _, q := range params {
				qm, _ := q.(map[string]any)
				found = found || (qm["in"] == "path" && qm["name"] == p[1])
			}
			if !found {
				params = append(params, map[string]any{"name": p[1], "in": "path", "required": true, "schema": map[string]any{"type": "string"}})
			}
		}
		if len(params) > 0 {
			op["parameters"] = params
		}
		ops, _ := paths[path].(map[string]any)
		if ops == nil {
			ops = map[string]any{}
			paths[path] = ops
		}
		ops[m] = op
	}
	doc["paths"] = paths
	scopes := map[string]any{}
	for _, sc := range accounts.Scopes {
		scopes[sc] = accounts.ScopeNotes[sc]
	}
	doc["x-holyarrs-scopes"] = scopes
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestOpenAPIGenerated is the API contract check: openapi.json must be
// what the code generates. A route, permission, scope or part changed in
// the code without regenerating (and committing) the spec fails here, and
// every operation needs a summary and answers.
func TestOpenAPIGenerated(t *testing.T) {
	t.Parallel()
	got := generateSpec(t)
	openapiJSON := readSpec(t)
	if *updateSpec {
		if err := os.WriteFile(specFile, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	if !bytes.Equal(bytes.ReplaceAll(openapiJSON, []byte("\r\n"), []byte("\n")), got) {
		t.Fatal("openapi.json does not match the code: run go generate ./cmd/holyarrs, read the diff and commit it with the change")
	}
	var doc struct {
		OpenAPI string                          `json:"openapi"`
		Paths   map[string]map[string]openapiOp `json:"paths"`
	}
	if err := json.Unmarshal(got, &doc); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(doc.OpenAPI, "3.") {
		t.Fatalf("openapi version %q", doc.OpenAPI)
	}
	for path, ops := range doc.Paths {
		for method, op := range ops {
			if op.Summary == "" || len(op.Responses) == 0 {
				t.Errorf("%s %s: needs a summary and responses", strings.ToUpper(method), path)
			}
		}
	}
}

// route: GET /api/v1/openapi.json
func TestOpenAPIServed(t *testing.T) {
	t.Parallel()
	srv, _, _ := setup(t)
	b := do(t, "GET", srv.URL+"/api/v1/openapi.json", "", 200)
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil || doc["paths"] == nil {
		t.Fatalf("served %s", b[:min(len(b), 200)])
	}
}

// specFile is the spec api embeds and serves.
const specFile = "openapi.json"

func readSpec(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(specFile)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
