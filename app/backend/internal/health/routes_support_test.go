package health

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

func bundleFiles(t *testing.T, h *AppHandlers) (*httptest.ResponseRecorder, map[string]string) {
	t.Helper()
	mux := http.NewServeMux()
	h.Routes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/support/bundle", nil))
	if rec.Code != 200 {
		t.Fatalf("bundle: %d %s", rec.Code, rec.Body)
	}
	b := rec.Body.Bytes()
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(rc)
		rc.Close()
		out[f.Name] = string(body)
	}
	return rec, out
}

func TestSupportBundleWithoutLog(t *testing.T) {
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	rec, files := bundleFiles(t, &AppHandlers{Store: s, Version: "t",
		HealthView: func(context.Context) any { return map[string]string{"a": "ok"} }})
	if rec.Header().Get("Content-Type") != "application/zip" || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("headers %v", rec.Header())
	}
	if !strings.Contains(files["log.txt"], "keeps no log file") {
		t.Fatalf("log.txt %q", files["log.txt"])
	}
	if !strings.Contains(files["health.json"], `"a": "ok"`) || files["about.json"] == "" || files["doctor.txt"] == "" {
		t.Fatalf("files %v", files)
	}
	// A bundle with no health view still downloads.
	_, files = bundleFiles(t, &AppHandlers{Store: s, Version: "t"})
	if _, ok := files["log.txt"]; !ok {
		t.Fatalf("files %v", files)
	}
}
