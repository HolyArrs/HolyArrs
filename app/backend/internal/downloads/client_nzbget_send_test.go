package downloads

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// An append NZBGet answers with an error, and that adds nothing, is a refusal:
// the Release may go elsewhere or be sent again, and it never waits as unknown.
func TestNZBGetAnsweredRefusalIsDefinite(t *testing.T) {
	f, base := startNZBGet(t)
	f.appendRefusal = true
	n := NewNZBGet(base, f.user, f.pass, nil)
	_, err := n.Add(context.Background(), AddRequest{URL: nzbSource(t), Name: "refused", Category: "tv", AllowLocal: true,
		AttemptMarker: "nzbget-refused-marker"})
	if err == nil || errors.Is(err, ErrUncertain) {
		t.Fatalf("answered refusal: %v", err)
	}
	if !strings.Contains(err.Error(), "Invalid parameter") || strings.Contains(err.Error(), f.pass) {
		t.Fatalf("the reply should name NZBGet's answer without the password: %v", err)
	}
}

// A refusing reply from a proxy in front of NZBGet (5xx) may hide an append that
// is still being read, so it stays unknown until NZBGet's lists say otherwise.
func TestNZBGetProxyFailureStaysUnknown(t *testing.T) {
	f, base := startNZBGet(t)
	f.appendHTTPError, f.hideAppend = true, true
	n := NewNZBGet(base, f.user, f.pass, nil)
	_, err := n.Add(context.Background(), AddRequest{URL: nzbSource(t), Name: "proxy", Category: "tv", AllowLocal: true,
		AttemptMarker: "nzbget-proxy-marker"})
	if !errors.Is(err, ErrUncertain) {
		t.Fatalf("5xx should stay unknown: %v", err)
	}
}

// A look-up NZBGet answers without the marker says so, so the send can be
// settled once its grace is over.
func TestNZBGetReconcileSaysNotArrived(t *testing.T) {
	f, base := startNZBGet(t)
	n := NewNZBGet(base, f.user, f.pass, nil)
	_, err := n.Add(context.Background(), AddRequest{Name: "gone", Category: "tv", AttemptMarker: "nzbget-gone", ReconcileOnly: true})
	if !errors.Is(err, ErrUncertain) || !errors.Is(err, ErrNotArrived) {
		t.Fatalf("absent marker: %v", err)
	}
	f.mu.Lock()
	f.rpcError = true
	f.mu.Unlock()
	_, err = n.Add(context.Background(), AddRequest{Name: "gone", Category: "tv", AttemptMarker: "nzbget-gone", ReconcileOnly: true})
	if errors.Is(err, ErrNotArrived) {
		t.Fatalf("a failed look-up must not count as absent: %v", err)
	}
}

// NZBGet's own answer (an RPC error or a 4xx) is final; no answer, a time-out,
// a 5xx, 408 or 429 is not.
func TestNZBGetAnsweredClassification(t *testing.T) {
	if !nzbgetAnswered(nzbgetReply{"append", "x"}) || !nzbgetAnswered(adapterHTTPError{413}) ||
		nzbgetAnswered(adapterHTTPError{502}) || nzbgetAnswered(adapterHTTPError{429}) || nzbgetAnswered(errDownloaderUnavailable) {
		t.Fatal("answered classification")
	}
}
