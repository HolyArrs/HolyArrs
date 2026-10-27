package metadata

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type gateNext struct {
	mu   sync.Mutex
	sent int
	code int
}

func (n *gateNext) RoundTrip(req *http.Request) (*http.Response, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.sent++
	code := n.code
	if code == 0 {
		code = 200
	}
	h := http.Header{}
	if code == 429 {
		h.Set("Retry-After", "60")
	}
	return &http.Response{StatusCode: code, Header: h, Body: io.NopCloser(strings.NewReader("{}")), Request: req}, nil
}

func gateReq(t *testing.T, ctx context.Context) *http.Request {
	t.Helper()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.example/v1/x", nil)
	return req
}

// Background requests keep a reserve for people, pace themselves, and are
// never sent while the server refuses; a 429 holds people briefly.
func TestGateBudgetAndRefusal(t *testing.T) {
	now := time.Unix(1000, 0)
	next := &gateNext{}
	g := &Gate{Next: next, Now: func() time.Time { return now }}
	ahead := Ahead(context.Background())

	// Background: the burst only, then it waits (a short ctx gives up).
	sent := 0
	for i := 0; i < 10; i++ {
		ctx, cancel := context.WithTimeout(ahead, 20*time.Millisecond)
		if _, err := g.RoundTrip(gateReq(t, ctx)); err == nil {
			sent++
		}
		cancel()
	}
	if sent != int(gateAheadBurst) {
		t.Fatalf("background burst sent %d, want %d", sent, int(gateAheadBurst))
	}
	// People still have the rest of the bucket, past the background reserve.
	for i := 0; i < int(gateBurst-gateAheadBurst); i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		if _, err := g.RoundTrip(gateReq(t, ctx)); err != nil {
			t.Fatalf("person request %d waited: %v", i, err)
		}
		cancel()
	}
	// A 429 pauses background requests: answered here, never sent.
	now = now.Add(time.Hour)
	next.code = 429
	if resp, err := g.RoundTrip(gateReq(t, context.Background())); err != nil || resp.StatusCode != 429 {
		t.Fatalf("429 not passed on: %v", err)
	}
	next.code = 0
	before := next.sent
	resp, err := g.RoundTrip(gateReq(t, ahead))
	if err != nil || resp.StatusCode != 429 || resp.Header.Get(GateHeld) == "" || resp.Header.Get("Retry-After") == "" || next.sent != before {
		t.Fatalf("background during a refusal: %v %v sent %d", resp, err, next.sent-before)
	}
	// People wait out the 429's Retry-After (at most gatePersonHoldMax).
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := g.RoundTrip(gateReq(t, ctx)); err == nil {
		t.Fatal("person request sent into a 429 hold")
	}
	now = now.Add(gatePersonHoldMax + time.Second)
	if _, err := g.RoundTrip(gateReq(t, context.Background())); err != nil {
		t.Fatalf("person after the hold: %v", err)
	}
	// Background stays paused for the whole Retry-After (60 s).
	if resp, _ := g.RoundTrip(gateReq(t, ahead)); resp.Header.Get(GateHeld) == "" {
		t.Fatal("background resumed before Retry-After")
	}
	now = now.Add(time.Minute)
	if resp, err := g.RoundTrip(gateReq(t, ahead)); err != nil || resp.Header.Get(GateHeld) != "" {
		t.Fatalf("background after Retry-After: %v", err)
	}
}
