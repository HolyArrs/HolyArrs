package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// route: GET /api/v1/explore/title-stats
// Rows carry a title layer; shown rows, opens and asks are counted per
// layer, the owner reads them back, and titles hold on a second load.
func TestExploreTitleStatsRoute(t *testing.T) {
	t.Parallel()
	a, _, _ := exploreRig(t)
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	get := func(path string) map[string]any {
		t.Helper()
		resp, err := http.Get(srv.URL + path)
		if err != nil || resp.StatusCode != 200 {
			t.Fatalf("%s: %v %v", path, err, resp.StatusCode)
		}
		defer resp.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return out
	}
	if err := a.explore.BuildPool(context.Background(), 1, "all"); err != nil {
		t.Fatal(err)
	}
	titles := func() []string {
		var out []string
		for _, path := range []string{"/api/v1/explore/head?type=all&tz=Europe/Paris", "/api/v1/explore/tail?type=all&page=0&tz=Europe/Paris"} {
			body := get(path)
			rows, _ := body["head"].([]any)
			if rows == nil {
				rows, _ = body["rows"].([]any)
			}
			for _, r := range rows {
				m := r.(map[string]any)
				if m["title_layer"] == nil || m["title_layer"] == "" {
					t.Fatalf("row %v has no title layer", m["id"])
				}
				out = append(out, m["title"].(string))
			}
		}
		return out
	}
	first := titles()
	if again := titles(); strings.Join(first, "|") != strings.Join(again, "|") {
		t.Fatalf("titles changed between loads:\n%v\n%v", first, again)
	}
	resp, err := http.Post(srv.URL+"/api/v1/explore/counts", "application/json", strings.NewReader(`{"row_type":"hidden","event":"open","title_layer":"genre"}`))
	if err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("count with layer: %v %v", err, resp.StatusCode)
	}
	resp.Body.Close()
	resp, _ = http.Post(srv.URL+"/api/v1/explore/counts", "application/json", strings.NewReader(`{"row_type":"hidden","event":"open","title_layer":"nope"}`))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad layer = %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()
	stats := get("/api/v1/explore/title-stats")
	layers := stats["layers"].([]any)
	shown, opens := 0.0, 0.0
	for _, l := range layers {
		m := l.(map[string]any)
		shown += m["shown"].(float64)
		opens += m["opens"].(float64)
	}
	if shown == 0 || opens != 1 || stats["templates"].(float64) < 450 {
		t.Fatalf("stats %v", stats)
	}
}
