package metadata

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func refusalLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	ResetRefusal()
	var buf bytes.Buffer
	prev, flags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() { log.SetOutput(prev); log.SetFlags(flags); ResetRefusal() })
	return &buf
}

// The server's reason is kept with its wait, and a hold is logged once,
// not once per refused request.
func TestRefusalKeptAndLoggedOncePerHold(t *testing.T) {
	buf := refusalLogs(t)
	now := time.Unix(5000, 0)
	body := []byte(`{"error":{"code":"registration_limit","retry_after_seconds":720}}`)
	for range 20 {
		noteRefusal(http.StatusTooManyRequests, "720", body, now)
	}
	r, ok := CurrentRefusal(now.Add(time.Minute))
	if !ok || r.Code != "registration_limit" || !r.Until.Equal(now.Add(720*time.Second)) {
		t.Fatalf("refusal = %+v %v", r, ok)
	}
	if n := strings.Count(buf.String(), "\n"); n != 1 {
		t.Fatalf("want one log line per hold, got %d:\n%s", n, buf.String())
	}
	if !strings.Contains(buf.String(), "registration_limit") {
		t.Fatalf("log names no reason: %q", buf.String())
	}
	if _, ok := CurrentRefusal(now.Add(721 * time.Second)); ok {
		t.Fatal("refusal outlived its Retry-After")
	}
	// A new hold after the old one ended logs again.
	noteRefusal(http.StatusServiceUnavailable, "5", []byte(`{"error":{"code":"upstream_busy"}}`), now.Add(800*time.Second))
	if n := strings.Count(buf.String(), "\n"); n != 2 {
		t.Fatalf("a new hold should log once more, got %d", n)
	}
}

// Background requests are not sent before a long Retry-After ends (a
// registration limit of an hour included), and every refusal is recorded.
func TestGateHoldsBackgroundForTheWholeRetryAfter(t *testing.T) {
	refusalLogs(t)
	var sent atomic.Int32
	now := time.Unix(10000, 0)
	g := &Gate{Now: func() time.Time { return now }, Next: refusalRT(func(r *http.Request) (*http.Response, error) {
		sent.Add(1)
		b := `{"error":{"code":"registry_full","retry_after_seconds":3600}}`
		h := http.Header{"Retry-After": {"3600"}}
		return &http.Response{StatusCode: 429, Header: h, Body: io.NopCloser(strings.NewReader(b)), Request: r}, nil
	})}
	req, _ := http.NewRequestWithContext(Ahead(context.Background()), "GET", "https://x/v1/tmdb/movie/1", nil)
	resp, err := g.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := io.ReadAll(resp.Body); !strings.Contains(string(b), "registry_full") {
		t.Fatalf("the caller must still read the server's answer: %q", b)
	}
	for _, later := range []time.Duration{time.Minute, 15 * time.Minute, 59 * time.Minute} {
		now = time.Unix(10000, 0).Add(later)
		resp, err := g.RoundTrip(req.Clone(req.Context()))
		if err != nil || resp.Header.Get(GateHeld) == "" {
			t.Fatalf("at %v a background request was sent", later)
		}
	}
	if sent.Load() != 1 {
		t.Fatalf("sent %d, want 1", sent.Load())
	}
	if r, ok := CurrentRefusal(now); !ok || r.Code != "registry_full" {
		t.Fatalf("refusal = %+v %v", r, ok)
	}
}

func TestBusyLineNamesTheReason(t *testing.T) {
	refusalLogs(t)
	now := time.Now()
	if line, _ := BusyLine(now); line != ErrServiceBusy.Error() {
		t.Fatalf("no refusal: %q", line)
	}
	noteRefusal(429, "600", []byte(`{"error":{"code":"registration_limit"}}`), now)
	line, secs := BusyLine(now)
	if !strings.Contains(line, "/config") || secs < 590 || secs > 600 {
		t.Fatalf("registration_limit: %q %d", line, secs)
	}
	for _, code := range []string{"client_limit", "registry_full", "upstream_busy", "service_busy", "installation_limit"} {
		ResetRefusal()
		noteRefusal(429, "30", []byte(`{"error":{"code":"`+code+`"}}`), now)
		if l, _ := BusyLine(now); l == "" || l == ErrServiceBusy.Error() {
			t.Fatalf("%s has no own line", code)
		}
	}
}

type refusalRT func(*http.Request) (*http.Response, error)

func (f refusalRT) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// A short refusal inside a longer hold keeps the longer hold's reason.
func TestShortRefusalKeepsLongerReason(t *testing.T) {
	refusalLogs(t)
	now := time.Unix(9000, 0)
	noteRefusal(429, "3600", []byte(`{"error":{"code":"registration_limit"}}`), now)
	noteRefusal(503, "1", []byte(`{"error":{"code":"service_busy"}}`), now.Add(5*time.Second))
	noteRefusal(429, "2", nil, now.Add(6*time.Second))
	if r, _ := CurrentRefusal(now.Add(10 * time.Second)); r.Code != "registration_limit" {
		t.Fatalf("code = %q", r.Code)
	}
}
