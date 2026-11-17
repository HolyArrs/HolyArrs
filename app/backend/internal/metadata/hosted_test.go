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
	regHoldOver(s)
	status.Store(503)
	if _, err := s.Register(context.Background()); !errors.Is(err, ErrServiceUnavailable) {
		t.Fatalf("503: %v", err)
	}
}

// regHoldOver ends the registration hold and returns how long was left.
func regHoldOver(s *Service) time.Duration {
	s.regHold.mu.Lock()
	defer s.regHold.mu.Unlock()
	left := time.Until(s.regHold.until)
	s.regHold.until = time.Now().Add(-time.Second)
	return left
}

// Any failed registration holds the next one: 30 s, doubling to 15 minutes,
// and back to 30 s after one that worked. Calls inside the hold get the calm
// busy answer without asking the server.
func TestServiceRegisterBacksOffAfterAnyFailure(t *testing.T) {
	var calls, status atomic.Int32
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(int(status.Load()))
		if status.Load() == 201 {
			_, _ = w.Write([]byte(`{"schema_version":1,"` + "tok" + `en":"` + fixtureValue + `"}`))
		}
	})
	near := func(got, want time.Duration) bool { return got > want-5*time.Second && got <= want }
	want := 30 * time.Second
	for i, code := range []int{429, 403, 500, 503, 404, 429, 500} {
		status.Store(int32(code))
		if _, err := s.Register(context.Background()); err == nil {
			t.Fatalf("%d: no error", code)
		}
		n := calls.Load()
		if _, err := s.Register(context.Background()); !errors.Is(err, ErrServiceBusy) || calls.Load() != n {
			t.Fatalf("%d: asked again inside the hold: %v", code, err)
		}
		if left := regHoldOver(s); !near(left, want) {
			t.Fatalf("failure %d (%d): held for %v, want %v", i+1, code, left, want)
		}
		want = min(want*2, 15*time.Minute)
	}
	if want != 15*time.Minute {
		t.Fatalf("the hold never reached its top: %v", want)
	}
	status.Store(201)
	if tok, err := s.Register(context.Background()); err != nil || tok != fixtureValue {
		t.Fatalf("after the hold: %q %v", tok, err)
	}
	status.Store(500)
	_, _ = s.Register(context.Background())
	if left := regHoldOver(s); !near(left, 30*time.Second) {
		t.Fatalf("after a success the hold starts over: %v", left)
	}
}

// A server that cannot be reached holds too; a caller that gave up does not.
func TestServiceRegisterBacksOffWhenUnreachable(t *testing.T) {
	var calls atomic.Int32
	s := testService(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) })
	gone, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Register(gone); err == nil {
		t.Fatal("cancelled call worked")
	}
	if s.regHold.held(time.Now()) {
		t.Fatal("a caller that gave up started a hold")
	}
	s.http = &http.Client{Transport: failingTransport{}}
	if _, err := s.Register(context.Background()); err == nil || errors.Is(err, ErrServiceBusy) {
		t.Fatalf("unreachable: %v", err)
	}
	if _, err := s.Register(context.Background()); !errors.Is(err, ErrServiceBusy) {
		t.Fatalf("unreachable server asked again inside the hold: %v", err)
	}
	if left := regHoldOver(s); left < 25*time.Second || left > 30*time.Second {
		t.Fatalf("held for %v, want 30s", left)
	}
}

type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("dial tcp: connection refused")
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

type offTransport struct{}

func (offTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, ErrSourceOff }

// With lookups off the server is never asked, so nothing failed: no hold,
// and each call keeps the quiet "off" answer instead of the busy one.
func TestServiceRegisterOffStartsNoHold(t *testing.T) {
	s := testService(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) })
	s.http = &http.Client{Transport: offTransport{}}
	for range 2 {
		if _, err := s.Register(context.Background()); !errors.Is(err, ErrSourceOff) {
			t.Fatalf("off: %v", err)
		}
	}
	if s.regHold.held(time.Now()) {
		t.Fatal("lookups off started a hold")
	}
}

// Picking a mode in Settings starts the registration wait over, so turning
// the server back on asks it straight away.
func TestModeChangeEndsTheRegistrationWait(t *testing.T) {
	var calls atomic.Int32
	s := testService(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) })
	_, _ = s.Register(context.Background())
	m := NewModes(func(string) string { return "" })
	m.OnSet = s.ResetRegisterWait
	if err := m.Set(ModeOff); err != nil {
		t.Fatal(err)
	}
	if err := m.Set(ModeServer); err != nil {
		t.Fatal(err)
	}
	n := calls.Load()
	if _, err := s.Register(context.Background()); errors.Is(err, ErrServiceBusy) || calls.Load() != n+1 {
		t.Fatalf("after the mode change the server was not asked: %v", err)
	}
}
