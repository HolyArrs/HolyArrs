//go:build lab

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// Connect Jackett or Prowlarr against the lab's own Jackett and
// Prowlarr, which hold nothing but the lab's fake indexer. Keys come from the
// environment only (labclients.py reads them from the lab apps' own config)
// and are never printed.
//
//	HOLYARRS_LAB_PROWLARR_KEY=… HOLYARRS_LAB_JACKETT_KEY=… go test -tags lab -run TestLabBridge ./cmd/holyarrs/

// TestLabBridgeProwlarr: the lab Prowlarr gets HolyArrs as an app on both
// sides, tests it live (it calls HolyArrs back through the Docker host),
// and its sync writes the fake indexer in as Sources; disconnecting takes
// HolyArrs out of Prowlarr's apps again, and its next sync removes them.
func TestLabBridgeProwlarr(t *testing.T) {
	key := os.Getenv("HOLYARRS_LAB_PROWLARR_KEY")
	if key == "" {
		t.Skip("HOLYARRS_LAB_PROWLARR_KEY not set")
	}
	prowlarr := envOr("HOLYARRS_LAB_PROWLARR_URL", "http://127.0.0.1:18704")
	f := newOldAppFix(t)
	// A second listener on every address, so Prowlarr's container can call HolyArrs back.
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	back := httptest.NewUnstartedServer(f.srv.Config.Handler)
	back.Listener = ln
	back.Start()
	t.Cleanup(back.Close)
	callback := fmt.Sprintf("http://%s:%d", envOr("HOLYARRS_LAB_HOST", "host.docker.internal"), ln.Addr().(*net.TCPAddr).Port)
	p := sources.NewBridgeProwlarr(prowlarr, key)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		_ = p.Disconnect(ctx, []string{callback + "/sonarr", callback + "/radarr"})
	})

	body := fmt.Sprintf(`{"kind":"prowlarr","url":%q,"key":%q,"callback_url":%q}`, prowlarr, key, callback)
	if st, out := f.session("POST", "/api/v1/bridges/test", body); st != 200 || !strings.Contains(out, "Prowlarr ") {
		t.Fatalf("test %d %s", st, out)
	}
	st, out := f.session("POST", "/api/v1/bridges", body)
	if st != 201 {
		t.Fatalf("connect %d %s", st, out)
	}
	var v sources.BridgeView
	_ = json.Unmarshal([]byte(out), &v)
	// Prowlarr's sync runs in the background there: wait for its Sources.
	deadline := time.Now().Add(90 * time.Second)
	for v.Sources < 2 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		_, out = f.session("GET", "/api/v1/bridges", "")
		var list struct {
			Bridges []sources.BridgeView `json:"bridges"`
		}
		_ = json.Unmarshal([]byte(out), &list)
		if len(list.Bridges) == 1 {
			v = list.Bridges[0]
		}
	}
	if v.Sources < 2 {
		t.Fatalf("Prowlarr wrote %d Sources, want one per side at least: %s", v.Sources, out)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	got, err := p.Connected(ctx, map[string]string{"Sonarr": callback + "/sonarr", "Radarr": callback + "/radarr"})
	if err != nil || !got["Sonarr"] || !got["Radarr"] {
		t.Fatalf("connected in Prowlarr: %v %v", got, err)
	}
	t.Logf("Prowlarr wrote %d Sources into HolyArrs", v.Sources)

	if st, out := f.session("DELETE", fmt.Sprintf("/api/v1/bridges/%d", v.ID), ""); st != 200 {
		t.Fatalf("disconnect %d %s", st, out)
	}
	got, _ = p.Connected(ctx, map[string]string{"Sonarr": callback + "/sonarr", "Radarr": callback + "/radarr"})
	if got["Sonarr"] || got["Radarr"] {
		t.Fatalf("still in Prowlarr's apps: %v", got)
	}
	var left int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM sources WHERE json_extract(extra, '$.old_app') IS NOT NULL").Scan(&left)
	if left != 0 {
		t.Fatalf("%d of Prowlarr's Sources stayed after disconnect", left)
	}
}
