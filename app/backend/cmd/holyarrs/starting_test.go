package main

import (
	"context"
	"database/sql"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

func TestStartingPageAnswersHealth(t *testing.T) {
	srv := httptest.NewServer(startingHandler(nil))
	defer srv.Close()
	for p, want := range map[string]int{"/healthz": 200, "/readyz": 503, "/library": 503} {
		resp, err := http.Get(srv.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Fatalf("%s: %d, want %d", p, resp.StatusCode, want)
		}
		if p == "/library" && !strings.Contains(string(b), "is starting") {
			t.Fatalf("page %q", b)
		}
	}
}

// Data from a newer build: the app keeps listening, says why and reports
// unhealthy, instead of exiting into a restart loop.
func TestNewerDataKeepsAPlainPage(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, database.MainFile))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL);
		INSERT INTO schema_migrations VALUES (9999, '9999_future.sql', 't')`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	_ = l.Close()
	c, err := parseFlags([]string{"-config", dir, "-addr", addr}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, c, runOpts{}) }()
	var resp *http.Response
	for i := 0; i < 100; i++ {
		if resp, err = http.Get("http://" + addr + "/healthz"); err == nil && resp.StatusCode == 503 {
			break
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil || resp.StatusCode != 503 {
		t.Fatalf("healthz: %v %v", resp, err)
	}
	resp.Body.Close()
	resp, err = http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), "from a newer HolyArrs. Run that version") {
		t.Fatalf("page %q", b)
	}
	cancel()
	if err := <-done; err == nil {
		t.Fatal("serve ended without the error")
	}
}

// Health says updating only while an update of the data runs.
func TestStartingPageSaysUpdatingOnlyWhenUpdating(t *testing.T) {
	var updating atomic.Bool
	srv := httptest.NewServer(startingHandler(&updating))
	defer srv.Close()
	get := func(p string) string {
		resp, err := http.Get(srv.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}
	if h, pg := get("/healthz"), get("/"); !strings.Contains(h, `"starting"`) || strings.Contains(pg, "updating") {
		t.Fatalf("before: %q %q", h, pg)
	}
	updating.Store(true)
	if h, pg := get("/healthz"), get("/"); !strings.Contains(h, `"updating"`) || !strings.Contains(pg, "updating its data") {
		t.Fatalf("during: %q %q", h, pg)
	}
}

func TestGoBackWordingFitsDocker(t *testing.T) {
	for _, s := range []string{goBack, restoreUsage} {
		for _, want := range []string{"docker exec <container> /holyarrs restore --pre-update", "docker restart <container>", "stop HolyArrs, run holyarrs restore --pre-update, start it again"} {
			if !strings.Contains(strings.ReplaceAll(s, "\n", " "), want) {
				t.Errorf("%q lacks %q", s, want)
			}
		}
		if strings.Contains(s, "stop HolyArrs first") {
			t.Errorf("%q asks Docker users to stop first", s)
		}
	}
}
