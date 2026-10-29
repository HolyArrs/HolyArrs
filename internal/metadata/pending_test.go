package metadata

import (
	"errors"
	"testing"
)

func TestPendingFromReadsWait(t *testing.T) {
	err := pendingFrom([]byte(`{"state":"queued","position":4,"message":"Queued, number 4 in line"}`))
	var p *PendingError
	if !errors.As(err, &p) || p.State != "queued" || p.Position != 4 || p.Message != "Queued, number 4 in line" {
		t.Fatalf("got %#v", err)
	}
	if !errors.Is(err, ErrServicePending) {
		t.Fatal("must stay a pending error")
	}
	if err := pendingFrom([]byte(`{}`)); err != ErrServicePending {
		t.Fatalf("bare body: %v", err)
	}
}
