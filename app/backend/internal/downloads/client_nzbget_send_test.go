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
	_, err := n.Add(context.Background(), ownSource(AddRequest{URL: nzbSource(t), Name: "refused", Category: "tv", AllowLocal: true,
		AttemptMarker: "nzbget-refused-marker"}))
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
	_, err := n.Add(context.Background(), ownSource(AddRequest{URL: nzbSource(t), Name: "proxy", Category: "tv", AllowLocal: true,
		AttemptMarker: "nzbget-proxy-marker"}))
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

// NZBGet 26.3 stores each member of the {Name, Value} struct as its own
// parameter. The job is still found by its marker in the queue, in the list and
// when a lost send is settled.
func TestNZBGetFindsItsMarkerWhenNZBGetSplitsTheStruct(t *testing.T) {
	f, base := startNZBGet(t)
	f.flatten = true
	n := NewNZBGet(base, f.user, f.pass, nil)
	r := ownSource(AddRequest{URL: nzbSource(t), Name: "flattened", Category: "tv", OperationKey: "flattened", AllowLocal: true})
	id, err := n.Add(context.Background(), r)
	if err != nil || id == "" || f.adds != 1 {
		t.Fatalf("add: %q %v adds=%d", id, err, f.adds)
	}
	if j, err := n.Get(context.Background(), id); err != nil || j.ID != id {
		t.Fatalf("get: %+v %v", j, err)
	}
	if jobs, err := n.List(context.Background(), "tv"); err != nil || len(jobs) != 1 || jobs[0].ID != id {
		t.Fatalf("list: %+v %v", jobs, err)
	}
	if got, err := n.Add(context.Background(), AddRequest{Name: r.Name, Category: "tv", AttemptMarker: id, ReconcileOnly: true}); err != nil || got != id {
		t.Fatalf("reconcile: %q %v", got, err)
	}
	if again, err := n.Add(context.Background(), r); err != nil || again != id || f.adds != 1 {
		t.Fatalf("a second send must find the first: %q %v adds=%d", again, err, f.adds)
	}
}

func TestNZBGetMarkerShapes(t *testing.T) {
	const id = "nzbget-0123"
	for _, c := range []struct {
		ps   []nzbParam
		want string
	}{
		{[]nzbParam{{"HolyArrsID", id}}, id},
		{[]nzbParam{{"Name", "HolyArrsID"}, {"Value", id}, {"*Unpack:", "yes"}}, id},
		{[]nzbParam{{"Value", id}, {"Name", "HolyArrsID"}}, id},
		{[]nzbParam{{"Value", id}}, ""},
		{[]nzbParam{{"Name", "Other"}, {"Value", id}}, ""},
		{[]nzbParam{{"Name", "HolyArrsID"}, {"Value", "not-ours"}}, ""},
		{nil, ""},
	} {
		if got := nzbgetMarkerID(c.ps); got != c.want {
			t.Errorf("%v: %q, want %q", c.ps, got, c.want)
		}
	}
}
