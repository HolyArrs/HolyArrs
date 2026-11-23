package main

import (
	"bufio"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
)

// newServer wraps every request in stallGuard, so the wrapper must still
// let the live stream flush and let websockets take over the connection.
func TestStallGuardPassesFlushAndHijack(t *testing.T) {
	flushed := make(chan error, 1)
	srv := httptest.NewServer(stallGuard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := w.(http.Hijacker); !ok {
			t.Error("the guard hides Hijacker")
		}
		if _, ok := w.(http.Flusher); !ok {
			t.Error("the guard hides Flusher")
		}
		_, _ = w.Write([]byte("first"))
		flushed <- http.NewResponseController(w).Flush()
	})))
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("get: %v %v", resp, err)
	}
	resp.Body.Close()
	if err := <-flushed; err != nil {
		t.Fatalf("flush: %v", err)
	}

	// A handler can take the connection over through the wrapper.
	hijacked := make(chan error, 1)
	srv2 := httptest.NewServer(stallGuard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, rw, err := http.NewResponseController(w).Hijack()
		if err != nil {
			hijacked <- err
			return
		}
		defer c.Close()
		_, err = rw.WriteString("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
		if err == nil {
			err = rw.Flush()
		}
		hijacked <- err
	})))
	defer srv2.Close()
	resp, err = http.Get(srv2.URL)
	if err != nil {
		t.Fatalf("hijacked get: %v", err)
	}
	got, _ := bufio.NewReader(resp.Body).ReadString('k')
	resp.Body.Close()
	if got != "ok" {
		t.Fatalf("hijacked body %q", got)
	}
	if err := <-hijacked; err != nil {
		t.Fatalf("hijack: %v", err)
	}
}

// A picture between the app-wide limit and the picture limit reaches the
// picture handler instead of being refused early.
func TestLimitBodiesLetsLargerAvatarThrough(t *testing.T) {
	var got int64
	h := limitBodies(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, _ := io.Copy(io.Discard, r.Body)
		got = n
	}))
	size := int64(3 << 20)
	req := httptest.NewRequest(http.MethodPut, accounts.AvatarUploadPath, bytes.NewReader(make([]byte, size)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || got != size {
		t.Fatalf("avatar of %d bytes: status %d, read %d", size, rec.Code, got)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/other", bytes.NewReader(make([]byte, size)))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("other path: status %d, want 413", rec.Code)
	}
}
