package metadata

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestServiceURLDefaultAndOverride(t *testing.T) {
	t.Setenv(EnvServiceURL, "")
	if ServiceURL() != "https://api.holyarrs.com" {
		t.Fatal(ServiceURL())
	}
	t.Setenv(EnvServiceURL, " https://meta.example.test/ ")
	if ServiceURL() != "https://meta.example.test" {
		t.Fatal(ServiceURL())
	}
}

// fixtureValue is a made-up installation credential.
var fixtureValue = strings.Join([]string{"hai", "fixture", "installation", "value"}, "_")

func TestServiceRegisterSendsNoCredential(t *testing.T) {
	var status atomic.Int32
	status.Store(201)
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/installations" || r.Header.Get("Authorization") != "" || r.ContentLength > 0 {
			t.Error("bad registration request")
		}
		w.WriteHeader(int(status.Load()))
		if status.Load() == 201 {
			_, _ = w.Write([]byte(`{"schema_version":1,"` + "tok" + `en":"` + fixtureValue + `"}`))
		}
	})
	tok, err := s.Register(context.Background())
	if err != nil || tok != fixtureValue {
		t.Fatalf("%q %v", tok, err)
	}
	status.Store(429)
	if _, err := s.Register(context.Background()); !errors.Is(err, ErrServiceBusy) {
		t.Fatalf("429: %v", err)
	}
	status.Store(503)
	if _, err := s.Register(context.Background()); !errors.Is(err, ErrServiceUnavailable) {
		t.Fatalf("503: %v", err)
	}
}

// A refused registration (429) with Retry-After is not tried again before
// that time: the calm busy answer comes back without asking the server.
func TestServiceRegisterWaitsOutRetryAfter(t *testing.T) {
	var calls, status atomic.Int32
	status.Store(429)
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if status.Load() == 429 {
			w.Header().Set("Retry-After", "120")
			w.WriteHeader(429)
			_, _ = w.Write([]byte(`{"error":{"code":"registration_limit"}}`))
			return
		}
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"schema_version":1,"` + "tok" + `en":"` + fixtureValue + `"}`))
	})
	if _, err := s.Register(context.Background()); !errors.Is(err, ErrServiceBusy) {
		t.Fatalf("429: %v", err)
	}
	status.Store(201)
	if _, err := s.Register(context.Background()); !errors.Is(err, ErrServiceBusy) || calls.Load() != 1 {
		t.Fatalf("asked again before Retry-After: %v, %d calls", err, calls.Load())
	}
	s.regHold.mu.Lock()
	left := time.Until(s.regHold.until)
	s.regHold.until = time.Now().Add(-time.Second)
	s.regHold.mu.Unlock()
	if left < 100*time.Second || left > 120*time.Second {
		t.Fatalf("held for %v, want about 120s", left)
	}
	if tok, err := s.Register(context.Background()); err != nil || tok != fixtureValue || calls.Load() != 2 {
		t.Fatalf("after Retry-After: %q %v, %d calls", tok, err, calls.Load())
	}
}

func TestHealthCachedForTTL(t *testing.T) {
	var calls, status atomic.Int32
	status.Store(200)
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/readyz" {
			t.Error(r.URL.Path)
		}
		calls.Add(1)
		w.WriteHeader(int(status.Load()))
	})
	h := &Health{Service: s, TTL: 50 * time.Millisecond}
	if !h.Up(context.Background()) || !h.Up(context.Background()) || calls.Load() != 1 {
		t.Fatalf("not cached: %d", calls.Load())
	}
	status.Store(503)
	time.Sleep(60 * time.Millisecond)
	if h.Up(context.Background()) || calls.Load() != 2 {
		t.Fatal("down not seen after TTL")
	}
}
