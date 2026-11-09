package backups

import (
	"bytes"
	"encoding/json"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// route: POST /api/v1/backups
// route: GET /api/v1/backups/{name}
// route: PUT /api/v1/backups/settings
// route: POST /api/v1/backups/restore
func TestRoutes(t *testing.T) {
	m, _, _, _ := setup(t)
	restarted := make(chan struct{}, 1)
	mux := http.NewServeMux()
	m.Routes(mux, func() { restarted <- struct{}{} })
	srv := httptest.NewServer(webserver.RecordRouteHits(mux))
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/api/v1/backups", "", nil)
	if err != nil || resp.StatusCode != 201 {
		t.Fatalf("manual backup: %v %v", resp.StatusCode, err)
	}
	resp.Body.Close()
	l, _ := m.List()
	if len(l) == 0 {
		t.Fatal("no backup made")
	}
	resp, _ = http.Get(srv.URL + "/api/v1/backups/" + l[0].Name)
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "application/zip" {
		t.Fatalf("download: %d", resp.StatusCode)
	}
	resp.Body.Close()
	req, _ := http.NewRequest("PUT", srv.URL+"/api/v1/backups/settings", strings.NewReader(`{"preset":"light"}`))
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != 200 {
		t.Fatalf("preset: %d", resp.StatusCode)
	}
	b, _ := os.ReadFile(l[0].Path)
	resp, _ = http.Post(srv.URL+RestorePath, "application/zip", bytes.NewReader(b))
	if resp.StatusCode != 202 {
		t.Fatalf("restore: %d", resp.StatusCode)
	}
	<-restarted
	resp, _ = http.Post(srv.URL+RestorePath, "application/zip", strings.NewReader("junk"))
	if resp.StatusCode != 400 {
		t.Fatalf("junk restore: %d", resp.StatusCode)
	}
}

// route: GET /api/v1/backups
// route: DELETE /api/v1/backups/{name}
func TestListAndDeleteRoutes(t *testing.T) {
	m, _, _, _ := setup(t)
	mux := http.NewServeMux()
	m.Routes(mux, func() {})
	srv := httptest.NewServer(webserver.RecordRouteHits(mux))
	defer srv.Close()
	b, err := m.Create(t.Context(), KindManual)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get(srv.URL + "/api/v1/backups")
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("list: %v %v", resp, err)
	}
	var l struct {
		Backups []Info `json:"backups"`
		KeyNote string `json:"key_note"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&l)
	resp.Body.Close()
	if len(l.Backups) != 1 || l.Backups[0].Name != b.Name || l.KeyNote == "" {
		t.Fatalf("listed %+v", l)
	}
	for _, want := range []int{204, 404} {
		req, _ := http.NewRequest("DELETE", srv.URL+"/api/v1/backups/"+b.Name, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil || resp.StatusCode != want {
			t.Fatalf("delete: %v, want %d", resp.StatusCode, want)
		}
		resp.Body.Close()
	}
}

func TestSettingsRefusesUnknownPreset(t *testing.T) {
	m, _, _, _ := setup(t)
	mux := http.NewServeMux()
	m.Routes(mux, func() {})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	for _, body := range []string{`{"preset":"hourly"}`, `not json`} {
		req, _ := http.NewRequest("PUT", srv.URL+"/api/v1/backups/settings", strings.NewReader(body))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Fatalf("body %q: status %d, want 400", body, resp.StatusCode)
		}
	}
	if p, err := m.Preset(t.Context()); err != nil || p != Balanced {
		t.Fatalf("preset after refusals %q %v", p, err)
	}
}
