package musictrend

import (
	"testing"
	"time"
)

func TestPassWait(t *testing.T) {
	every := 6 * time.Hour
	if got := passWait(true, true, every); got != every {
		t.Fatalf("ready pass: %v", got)
	}
	if got := passWait(false, false, every); got != 10*time.Minute {
		t.Fatalf("never live: %v", got)
	}
	if got := passWait(false, true, every); got != time.Hour {
		t.Fatalf("live, pass open: %v", got)
	}
	if got := passWait(false, true, 30*time.Minute); got != 30*time.Minute {
		t.Fatalf("live, short every: %v", got)
	}
}
