package main

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/logs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/health"
)

// An admin who is not the owner reads the log viewer and the log download;
// neither may show an owner reset code, or the admin could take the owner
// over. The program's own output (docker logs) still shows it.
func TestOwnerResetCodeNotInAdminLogs(t *testing.T) {
	dir := t.TempDir()
	var out, code bytes.Buffer
	p := ownerPrompt{in: strings.NewReader("luis\na long phrase\n"), out: &out, code: &code}
	if rc := p.run([]string{"-config", dir}); rc != 0 {
		t.Fatalf("reset: %s", out.String())
	}

	applogs, err := logs.Open(filepath.Join(dir, "logs"))
	if err != nil {
		t.Fatal(err)
	}
	defer applogs.Close()
	var stdout bytes.Buffer
	prev := log.Writer()
	log.SetOutput(io.MultiWriter(applogs.Mask(&stdout), applogs)) // as main does
	defer log.SetOutput(prev)

	p.in = strings.NewReader("")
	if rc := p.run([]string{"-config", dir, "--code"}); rc != 0 {
		t.Fatalf("--code: %s", out.String())
	}
	got := strings.TrimSpace(code.String())
	if len(got) != 29 {
		t.Fatalf("code %q", got)
	}
	if !strings.Contains(stdout.String(), "OWNER RESET CODE: "+got) {
		t.Fatalf("container output lost the code: %q", stdout.String())
	}

	m := http.NewServeMux()
	(&health.AppHandlers{Version: "test", Started: time.Now(), Log: applogs, ConfigDir: dir}).Routes(m)
	srv := httptest.NewServer(m)
	defer srv.Close()
	for _, path := range []string{"/api/v1/logs?limit=500", "/api/v1/logs?text=OWNER", "/api/v1/logs/download"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("%s: %d %s", path, resp.StatusCode, b)
		}
		if !strings.Contains(string(b), "OWNER RESET CODE") {
			t.Fatalf("%s: the line itself should stay, masked: %s", path, b)
		}
		if strings.Contains(string(b), got) || strings.Contains(string(b), strings.ReplaceAll(got, "-", "")) {
			t.Fatalf("%s shows the owner reset code: %s", path, b)
		}
	}
}
