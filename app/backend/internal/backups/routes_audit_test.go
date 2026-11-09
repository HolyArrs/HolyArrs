package backups

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

func TestDownloadAndDeleteLeaveAuditEvents(t *testing.T) {
	m, s, _, _ := setup(t)
	mux := http.NewServeMux()
	m.Routes(mux, nil)
	srv := httptest.NewServer(webserver.RecordRouteHits(mux))
	defer srv.Close()
	b, err := m.Create(t.Context(), KindManual)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get(srv.URL + "/api/v1/backups/" + b.Name)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("download: %v %v", resp, err)
	}
	resp.Body.Close()
	req, _ := http.NewRequest("DELETE", srv.URL+"/api/v1/backups/"+b.Name, nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 204 {
		t.Fatalf("delete: %v %v", resp, err)
	}
	resp.Body.Close()
	evs, _, err := s.Events(context.Background(), database.EventFilter{Kinds: []string{"backup_used"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 2 {
		t.Fatalf("want 2 backup_used events, got %d: %+v", len(evs), evs)
	}
	got := map[string]bool{}
	for _, e := range evs {
		if e.Data["name"] != b.Name {
			t.Fatalf("event names %v, want %s", e.Data["name"], b.Name)
		}
		got[e.Data["action"].(string)] = true
	}
	if !got["downloaded"] || !got["deleted"] {
		t.Fatalf("actions %v", got)
	}
}

func TestBackupFileRoutesNeedStrongSignIn(t *testing.T) {
	m, _, _, dir := setup(t)
	mux := http.NewServeMux()
	m.Routes(mux, func() {})
	weak := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r.WithContext(webserver.ContextWith(r.Context(), webserver.Caller{UserID: 1, Via: "local_network"})))
	})
	srv := httptest.NewServer(weak)
	defer srv.Close()
	b, err := m.Create(t.Context(), KindManual)
	if err != nil {
		t.Fatal(err)
	}
	zip, _ := os.ReadFile(b.Path)
	for _, c := range []struct{ method, path, body string }{
		{"GET", "/api/v1/backups/" + b.Name, ""},
		{"DELETE", "/api/v1/backups/" + b.Name, ""},
		{"POST", RestorePath, string(zip)},
	} {
		req, _ := http.NewRequest(c.method, srv.URL+c.path, strings.NewReader(c.body))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 401 && resp.StatusCode != 403 {
			t.Fatalf("%s %s: %d, want 401/403", c.method, c.path, resp.StatusCode)
		}
	}
	if _, err := os.Stat(b.Path); err != nil {
		t.Fatalf("backup gone: %v", err)
	}
	if Staged(dir) {
		t.Fatal("restore staged by a weak caller")
	}
}
