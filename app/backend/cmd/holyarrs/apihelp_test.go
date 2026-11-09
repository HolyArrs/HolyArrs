package main

import (
	"testing"
)

// The jobs and imports lists answer JSON (their end-to-end test runs in
// cmd/holyarrs; this keeps the routes served here too).
//
// route: GET /api/v1/imports
// route: GET /api/v1/jobs
func TestJobsAndImportsServed(t *testing.T) {
	t.Parallel()
	srv, _, _ := setup(t)
	for _, p := range []string{"/api/v1/jobs", "/api/v1/imports"} {
		if b := do(t, "GET", srv.URL+p, "", 200); len(b) == 0 || b[0] != '{' && b[0] != '[' {
			t.Fatalf("%s: %s", p, b)
		}
	}
}
