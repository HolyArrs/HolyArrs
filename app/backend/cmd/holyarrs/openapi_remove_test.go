package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// Remove's answer carries the Downloader's refusal (client_note), and its
// 409 names the finished-download refusals, so a generated client keeps both.
func TestOpenAPIRemoveSaysClientNote(t *testing.T) {
	b, err := os.ReadFile("openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths map[string]map[string]struct {
			Responses map[string]json.RawMessage `json:"responses"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(b, &spec); err != nil {
		t.Fatal(err)
	}
	op := spec.Paths["/api/v1/downloads/{id}/remove"]["post"]
	if ok := string(op.Responses["200"]); !strings.Contains(ok, `"client_note"`) || !strings.Contains(ok, "DownloadView") {
		t.Fatalf("200 does not document client_note: %s", ok)
	}
	if c := string(op.Responses["409"]); !strings.Contains(c, "read-only") || !strings.Contains(c, "placing") {
		t.Fatalf("409 does not name the read-only and importing refusals: %s", c)
	}
}
