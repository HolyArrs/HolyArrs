package webserver

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServeAttachment(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file := filepath.Join(dir, "a.mp4")
	if err := os.WriteFile(file, []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	serve := func(p, name string, download bool, hdr map[string]string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/x", nil)
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		ServeAttachment(w, req, p, name, download)
		return w
	}

	w := serve(file, "a.mp4", false, map[string]string{"Range": "bytes=2-4"})
	if w.Code != http.StatusPartialContent || w.Body.String() != "234" || w.Header().Get("Content-Range") != "bytes 2-4/10" ||
		w.Header().Get("Content-Type") != "video/mp4" || w.Header().Get("Content-Disposition") != "" || w.Header().Get("Last-Modified") == "" {
		t.Fatalf("range: %d %q %v", w.Code, w.Body.String(), w.Header())
	}
	if cd := serve(file, "Plain name.mp4", true, nil).Header().Get("Content-Disposition"); cd != `attachment; filename="Plain name.mp4"` {
		t.Fatalf("ascii name: %q", cd)
	}
	if cd := serve(file, "Zażółć \"x\".mp4", true, nil).Header().Get("Content-Disposition"); cd != `attachment; filename*=utf-8''Za%C5%BC%C3%B3%C5%82%C4%87%20%22x%22.mp4` {
		t.Fatalf("utf-8 name: %q", cd)
	}
	// A type the caller set wins over the extension and the bytes.
	odd := filepath.Join(dir, "a.noext")
	if err := os.WriteFile(odd, []byte("plain text, not a video"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/x", nil)
	w = httptest.NewRecorder()
	w.Header().Set("Content-Type", "video/mp4")
	ServeAttachment(w, req, odd, "a.noext", false)
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "video/mp4" {
		t.Fatalf("preset type: %d %v", w.Code, w.Header())
	}
	for _, p := range []string{filepath.Join(dir, "gone.mp4"), dir} {
		w := serve(p, "gone.mp4", true, nil)
		if w.Code != http.StatusNotFound || w.Header().Get("Content-Disposition") != "" || strings.Contains(w.Body.String(), dir) {
			t.Fatalf("%s: %d %v %s", p, w.Code, w.Header(), w.Body.String())
		}
	}
}
