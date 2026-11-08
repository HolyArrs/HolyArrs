package downloads

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReleasePayloadPolicy(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("downloader credentials reached the source")
		}
		_, _ = w.Write([]byte("owned fixture"))
	}))
	defer srv.Close()
	if _, err := fetchRelease(context.Background(), srv.URL, false, 128); err == nil || calls != 0 {
		t.Fatal("local source reached without permission")
	}
	if b, err := fetchRelease(context.Background(), srv.URL, true, 128); err != nil || string(b) != "owned fixture" {
		t.Fatalf("allowed fixture: %q %v", b, err)
	}
	if _, err := fetchRelease(context.Background(), srv.URL, true, 4); err == nil {
		t.Fatal("accepted oversized release file")
	}
	for _, raw := range []string{"file:///etc/passwd", "http://user:private@host/", "http://169.254.169.254/", "http://metadata.google.internal/"} {
		_, err := fetchRelease(context.Background(), raw, true, 128)
		if err == nil || strings.Contains(err.Error(), "private") {
			t.Fatalf("unsafe address/error: %v", err)
		}
	}
}

func TestReleasePayloadRejectsRedirectAndHonorsCancellation(t *testing.T) {
	hits := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++ }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer redirect.Close()
	if _, err := fetchRelease(context.Background(), redirect.URL+"?apikey=payload-private", true, 128); err == nil || strings.Contains(err.Error(), "payload-private") || hits != 0 {
		t.Fatalf("redirect policy: %d %v", hits, err)
	}
	hung := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer hung.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := fetchRelease(ctx, hung.URL, true, 128); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline: %v", err)
	}
}
