package imgstore

import (
	"errors"
	"testing"
	"time"
)

// A source failing more than 20% of at least 20 answers in ten minutes is
// flagged; older answers drop out of the window.
func TestSourceAlertOverTenMinutes(t *testing.T) {
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	l := NewLimits(nil)
	l.Now = func() time.Time { return now }
	for range 16 {
		l.Done("deezer", 200, nil)
	}
	for range 3 {
		l.Done("deezer", 0, errors.New("timeout"))
	}
	l.Backoff("deezer", time.Second)
	st := l.Status()["deezer"]
	if st.Answers10m != 20 || st.Failed10m != 3 || st.Limited10m != 1 || st.Alert {
		t.Fatalf("4 of 20 failing is 20%%, not over it: %+v", st)
	}
	l.Done("deezer", 503, nil)
	if st := l.Status()["deezer"]; !st.Alert {
		t.Fatalf("5 of 21 failing is over 20%%: %+v", st)
	}
	now = now.Add(11 * time.Minute)
	if st := l.Status()["deezer"]; st.Answers10m != 0 || st.Alert {
		t.Fatalf("answers older than ten minutes drop out: %+v", st)
	}
}
