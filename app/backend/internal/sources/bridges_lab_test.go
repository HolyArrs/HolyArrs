//go:build lab

package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// TestLabBridgeJackett: the lab Jackett answers the site list with its key,
// turns a wrong key down the way the manual says (200 with an error), and
// the finder knows its sign-in page.
func TestLabBridgeJackett(t *testing.T) {
	key := os.Getenv("HOLYARRS_LAB_JACKETT_KEY")
	if key == "" {
		t.Skip("HOLYARRS_LAB_JACKETT_KEY not set")
	}
	jackett := envOr("HOLYARRS_LAB_JACKETT_URL", "http://127.0.0.1:18715")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	sites, err := NewBridgeJackett(jackett, key).Sites(ctx)
	if err != nil {
		t.Fatalf("site list: %v", err)
	}
	t.Logf("the lab Jackett has %d sites set up", len(sites))
	if len(sites) == 0 {
		t.Fatal("the lab Jackett has no site set up: set up the lab definition first")
	}
	if _, err := NewBridgeJackett(jackett, "not-the-key").Sites(ctx); err == nil || !strings.Contains(BridgeWords(err), "turned down the key") {
		t.Fatalf("wrong key: %v", err)
	}
	if !BridgeLooks(ctx, &http.Client{Timeout: 5 * time.Second}, BridgeKindJackett, jackett) {
		t.Fatal("the finder does not know the lab Jackett's sign-in page")
	}
	// Connected through the API: its sites become Sources (none, if it has none).
	srv, _ := sourceServer(t)
	var v BridgeView
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/bridges", fmt.Sprintf(`{"kind":"jackett","url":%q,"key":%q}`, jackett, key), 201), &v)
	if v.Sites != len(sites) || v.Sources != len(sites) {
		t.Fatalf("connect %+v, want %d sites", v, len(sites))
	}
	// Each Source it made answers through the real Jackett feed (the lab
	// site reads the fake indexer).
	var list struct {
		Sources []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"sources"`
	}
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/sources", "", 200), &list)
	for _, src := range list.Sources {
		out := string(do(t, "POST", fmt.Sprintf("%s/api/v1/sources/%d/test", srv.URL, src.ID), "", 200))
		if !strings.Contains(out, `"ok":true`) {
			t.Fatalf("%s through Jackett: %s", src.Name, out)
		}
		t.Logf("%s answered through Jackett", src.Name)
	}
}
func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
