package discover

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func TestWatchOnMarks(t *testing.T) {
	t.Parallel()
	a := &WatchOn{}
	var calls atomic.Int32
	done := make(chan struct{}, 4)
	a.Fetch = func(ctx context.Context, kind, id string) (map[string][]metadata.WatchService, error) {
		calls.Add(1)
		defer func() { done <- struct{}{} }()
		if id == "2" {
			return nil, errors.New("down")
		}
		return map[string][]metadata.WatchService{"US": {{ID: 15, Name: "Hulu"}, {ID: 8, Name: "Netflix"}}, "GB": {{ID: 9, Name: "Prime"}}}, nil
	}
	keys := []string{"movie:1", "show:2"}
	if m := a.Marks("US", []int{8, 15}, keys); len(m) != 0 {
		t.Fatalf("cold cache never waits: %+v", m)
	}
	for range 2 {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("background read did not run")
		}
	}
	time.Sleep(10 * time.Millisecond)
	m := a.Marks("US", []int{8, 15}, keys)
	if got := m["movie:1"]; got.Service != 8 || got.Name != "Netflix" {
		t.Fatalf("member's order wins: %+v", got)
	}
	if _, ok := m["show:2"]; ok {
		t.Fatal("failed read gives no mark")
	}
	if len(a.Marks("FR", []int{8}, keys)) != 0 || len(a.Marks("GB", []int{8}, keys)) != 0 {
		t.Fatal("region without the service: no mark")
	}
	if calls.Load() != 2 {
		t.Fatalf("one read per title, cached across regions; got %d", calls.Load())
	}
	// A day later the answer is read again.
	a.mu.Lock()
	for k, e := range a.titles {
		e.at = e.at.Add(-watchOnTTL)
		a.titles[k] = e
	}
	a.mu.Unlock()
	a.Marks("US", []int{8}, []string{"movie:1"})
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("expired title not read again")
	}
}
