package api

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// speedKeep is how many one-second readings of total speed are remembered.
const speedKeep = 120

// SpeedSample is one second of total download and upload speed.
type SpeedSample struct {
	Down int64 `json:"down"`
	Up   int64 `json:"up"`
}

// speedHistory is a rolling buffer of the total speeds, filled from app start.
type speedHistory struct {
	mu  sync.Mutex
	buf []SpeedSample
}

func (h *speedHistory) add(s SpeedSample) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.buf = append(h.buf, s)
	if len(h.buf) > speedKeep {
		h.buf = append([]SpeedSample(nil), h.buf[len(h.buf)-speedKeep:]...)
	}
}

// last returns up to n of the newest readings, oldest first.
func (h *speedHistory) last(n int) []SpeedSample {
	h.mu.Lock()
	defer h.mu.Unlock()
	b := h.buf
	if n > 0 && len(b) > n {
		b = b[len(b)-n:]
	}
	return append([]SpeedSample{}, b...)
}

// sampleSpeeds reads the total speeds once a second from the database (no outside calls).
func (a *API) sampleSpeeds() {
	if a.s.Store == nil {
		return
	}
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for range t.C {
		ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
		var s SpeedSample
		err := a.s.Store.Read().QueryRowContext(ctx, "SELECT COALESCE(SUM(speed),0), COALESCE(SUM(up_speed),0) FROM downloads WHERE state = 'downloading'").Scan(&s.Down, &s.Up)
		cancel()
		if err == nil {
			a.speeds.add(s)
		}
	}
}

func (a *API) speedHistory(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"samples": a.speeds.last(60)})
}
