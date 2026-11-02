package musictags

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A build that lost more than MaxShrink of the published items is not
// published: the old bundle keeps answering and the refusal is reported.
// One within the limit publishes.
func TestShrunkBundleRefused(t *testing.T) {
	var calls int64
	up := fakeUpstream(t, &calls)
	defer up.Close()
	now := fixedNow
	s, err := Open(filepath.Join(t.TempDir(), "musictags.sqlite"), Options{HTTP: up.Client(), MBHTTP: up.Client(), LBRoot: up.URL + "/1", MBRoot: up.URL + "/ws/2", UserAgent: "test", Now: func() time.Time { return now }, Charts: testCharts})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	built := s.publishedItems()
	if built == 0 {
		t.Fatal("no items built")
	}
	// Pretend the published bundle had far more items than a build makes.
	s.mu.Lock()
	s.items = built * 10
	old, oldTag := s.body, s.etag
	s.mu.Unlock()
	now = now.Add(25 * time.Hour)
	if err := s.Refresh(context.Background()); !errors.Is(err, ErrShrunk) {
		t.Fatalf("shrunk build: %v", err)
	}
	body, tag, _ := s.Pools()
	if string(body) != string(old) || tag != oldTag {
		t.Fatal("a shrunk bundle replaced the published one")
	}
	st := s.PublishStatus()
	if r, _ := st["refused"].(string); !strings.Contains(r, "fewer") || st["items"] != built*10 {
		t.Fatalf("status %v", st)
	}
	// Within 10%: published.
	s.mu.Lock()
	s.items = built + built/20
	s.mu.Unlock()
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatalf("a build within the limit was refused: %v", err)
	}
	if st := s.PublishStatus(); st["refused"] != nil || st["items"] != built {
		t.Fatalf("status after publish %v", st)
	}
}
