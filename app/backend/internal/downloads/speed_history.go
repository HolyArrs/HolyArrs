package downloads

import (
	"context"
	"database/sql"
	"net/http"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// speedKeep is how many one-second readings of total speed are remembered: 15 minutes.
const speedKeep = 900

// SpeedSample is one second of total download and upload speed.
type SpeedSample struct {
	Down int64 `json:"down"`
	Up   int64 `json:"up"`
}

// SpeedHistory is a rolling buffer of the total speeds, filled from app start.
type SpeedHistory struct {
	mu  sync.Mutex
	buf []SpeedSample
}

func (h *SpeedHistory) add(s SpeedSample) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.buf = append(h.buf, s)
	if len(h.buf) > speedKeep {
		h.buf = append([]SpeedSample(nil), h.buf[len(h.buf)-speedKeep:]...)
	}
}

// last returns up to n of the newest readings, oldest first.
func (h *SpeedHistory) last(n int) []SpeedSample {
	h.mu.Lock()
	defer h.mu.Unlock()
	b := h.buf
	if n > 0 && len(b) > n {
		b = b[len(b)-n:]
	}
	return append([]SpeedSample{}, b...)
}

// Start reads the total speeds once a second from db (no outside calls)
// until ctx ends. The program starts it once; tests do not.
func (h *SpeedHistory) Start(ctx context.Context, db *sql.DB) {
	go h.sample(ctx, db)
}

func (h *SpeedHistory) sample(ctx context.Context, db *sql.DB) {
	if db == nil {
		return
	}
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		ctx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
		s, err := totalSpeeds(ctx, db)
		cancel()
		if err == nil {
			h.add(s)
		}
	}
}

// Serve answers GET /api/v1/downloads/speeds.
func (h *SpeedHistory) Serve(w http.ResponseWriter, r *http.Request) {
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"samples": h.last(0)})
}
