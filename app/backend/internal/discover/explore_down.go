package discover

import (
	"errors"
	"log"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// downWaitMin and downWaitMax bound the wait after the metadata server did
// not answer an Explore list: it doubles per failure in a row. A slow spell
// leaves Explore on kept lists for two minutes at most.
const (
	downWaitMin = 30 * time.Second
	downWaitMax = 2 * time.Minute
)

// serverHold is the Explore wait while the metadata server is not
// answering: lists come from what is kept, and the state is logged once
// when it changes, not on every pass.
type serverHold struct {
	mu    sync.Mutex
	down  bool
	fails int
	until time.Time
}

// held says outside list asks wait now.
func (h *serverHold) held(now time.Time) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.down && now.Before(h.until)
}

// note records one outside ask's answer; err nil is an answer. Our own
// guard refusing at once says nothing new about the server (its own wait is
// already running), so it does not start or lengthen the wait.
func (h *serverHold) note(err error, now time.Time) {
	if errors.Is(err, outbound.ErrSuppressed) {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if err == nil {
		if h.down {
			log.Printf("explore: metadata server answers again")
		}
		h.down, h.fails, h.until = false, 0, time.Time{}
		return
	}
	if !h.down {
		log.Printf("explore: metadata server is not answering, Explore uses kept lists and tries again later: %v", err)
	}
	d := min(downWaitMin<<min(h.fails, 5), downWaitMax)
	h.down, h.fails, h.until = true, h.fails+1, now.Add(d)
}

// serverDown says err means the metadata server did not answer.
func serverDown(err error) bool { return errors.Is(err, metadata.ErrServiceUnavailable) }

// lookupsOff says the owner turned lookups off, so Explore asks no one.
func (e *Explore) lookupsOff() bool { return e.LookupsOff != nil && e.LookupsOff() }
