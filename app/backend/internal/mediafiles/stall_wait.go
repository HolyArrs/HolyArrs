package mediafiles

import (
	"time"
)

// StallReasonPrefix starts the reason of a Download failed for making no
// progress. Such a failure never turns its Release down.
const StallReasonPrefix = "no progress for "

// StallWait is how long a Release waits before it is sent again after n of
// its Downloads in a row failed for no progress: 1 day, then 2, 4, and at
// most 7. Zero for none.
func StallWait(n int) time.Duration {
	if n <= 0 {
		return 0
	}
	days := 7
	if n <= 3 {
		days = 1 << (n - 1)
	}
	return time.Duration(days) * 24 * time.Hour
}

// Stall is how a Release's Downloads stalled in a row: how many, and when
// the newest of them failed.
type Stall struct {
	Count int
	Last  time.Time
}

// Until is when the Release may be sent again; zero when it need not wait.
func (s Stall) Until() time.Time {
	if s.Count == 0 {
		return time.Time{}
	}
	return s.Last.Add(StallWait(s.Count))
}
