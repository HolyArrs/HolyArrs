package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCompanionReadsOnlyHealthAndSummarises(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Method != "GET" || r.URL.Path != "/base/api/v1/health/states" || r.Header.Get("X-Api-Key") != "synthetic-key" {
			t.Error("unexpected companion call")
		}
		_, _ = w.Write([]byte(`{"safe_mode":false,"states":[{"name":"private label","state":"running"},{"state":"retrying"}]}`))
	}))
	defer srv.Close()
	got, err := readStates(context.Background(), srv.URL+"/base", "synthetic-key", srv.Client())
	if err != nil || got["running"] != 1 || got["retrying"] != 1 || hits.Load() != 1 {
		t.Fatalf("counts %v err %v hits %d", got, err, hits.Load())
	}
}

func TestCompanionRefusesRedirectAndDoesNotLeakAnswer(t *testing.T) {
	var reached atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached.Store(true) }))
	defer target.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", target.URL)
		w.WriteHeader(http.StatusFound)
		_, _ = w.Write([]byte("synthetic-private-key"))
	}))
	defer srv.Close()
	_, err := readStates(context.Background(), srv.URL, "synthetic-private-key", srv.Client())
	if err == nil || reached.Load() || strings.Contains(err.Error(), "synthetic-private-key") || strings.Contains(err.Error(), srv.URL) {
		t.Fatalf("unsafe redirect/error: %v", err)
	}
}

func TestCompanionRefusesBadInputAndAnswers(t *testing.T) {
	for _, base := range []string{"file:///tmp/a", "http://user:pass@example.invalid", "http://example.invalid?key=bad", "http://example.invalid#bad"} {
		if _, err := readStates(context.Background(), base, "synthetic", http.DefaultClient); err == nil {
			t.Errorf("accepted %s", base)
		}
	}
	for _, body := range []string{`{}`, `{"states":[{"state":"unknown"}]}`, "broken", strings.Repeat("x", (1<<20)+1)} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
		_, err := readStates(context.Background(), srv.URL, "synthetic", srv.Client())
		srv.Close()
		if err == nil {
			t.Error("accepted an invalid answer")
		}
	}
}

func TestCompanionCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("canceled call reached app") }))
	defer srv.Close()
	if _, err := readStates(ctx, srv.URL, "synthetic", srv.Client()); err == nil {
		t.Fatal("canceled call succeeded")
	}
}
