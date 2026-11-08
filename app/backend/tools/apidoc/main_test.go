package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAPIDocUpToDate fails when docs/dev/api-routes.md no longer matches the
// route-table snapshot. Fix it with: go run ./app/backend/tools/apidoc
func TestAPIDocUpToDate(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	want, err := build(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(docRel)))
	if err != nil {
		t.Fatalf("%s missing: run go run ./app/backend/tools/apidoc (%v)", docRel, err)
	}
	if strings.ReplaceAll(string(got), "\r\n", "\n") != want {
		t.Fatalf("%s is out of date with the route-table snapshot: run go run ./app/backend/tools/apidoc", docRel)
	}
}

func TestRenderGroupsByArea(t *testing.T) {
	out := render([]route{
		{Method: "POST", Path: "/api/v1/asks", Auth: "ask", Purpose: "make one"},
		{Method: "GET", Path: "/api/v1/asks", Auth: "ask"},
		{Method: "GET", Path: "/movie", Auth: "unlisted"},
	})
	if !strings.Contains(out, "## asks") || !strings.Contains(out, "## other (outside /api/v1)") {
		t.Fatalf("missing sections:\n%s", out)
	}
	if strings.Index(out, "| GET | `/api/v1/asks`") > strings.Index(out, "| POST |") {
		t.Fatal("GET should come before POST")
	}
}
