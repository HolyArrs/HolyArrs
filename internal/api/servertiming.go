package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

// serverTiming collects the phases of one answer for the Server-Timing
// header, so a browser's network panel shows where a slow answer spent
// its time.
type serverTiming struct {
	last  time.Time
	start time.Time
	parts []string
}

func newServerTiming() *serverTiming {
	now := time.Now()
	return &serverTiming{last: now, start: now}
}

// mark ends the phase called name.
func (t *serverTiming) mark(name string) {
	now := time.Now()
	t.parts = append(t.parts, fmt.Sprintf("%s;dur=%.1f", name, float64(now.Sub(t.last).Microseconds())/1000))
	t.last = now
}

// write sets the header with every phase and the total.
func (t *serverTiming) write(w http.ResponseWriter) {
	all := append(t.parts, fmt.Sprintf("total;dur=%.1f", float64(time.Since(t.start).Microseconds())/1000))
	w.Header().Set("Server-Timing", strings.Join(all, ", "))
}
