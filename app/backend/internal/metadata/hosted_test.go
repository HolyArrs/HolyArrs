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
