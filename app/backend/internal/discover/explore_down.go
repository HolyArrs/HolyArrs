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
// already running), so it does not start or lengthen the wait. It says true
// when the server answers again after a spell down.
func (h *serverHold) note(err error, now time.Time) (back bool) {
	if errors.Is(err, outbound.ErrSuppressed) {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if err == nil {
		back = h.down
		if back {
			log.Printf("explore: metadata server answers again")
		}
		h.down, h.fails, h.until = false, 0, time.Time{}
		return back
	}
	until := now.Add(min(downWaitMin<<min(h.fails, 5), downWaitMax))
	if r, ok := metadata.CurrentRefusal(now); ok && serverBusy(err) {
		// The server said how long to wait (logged once on the metadata
		// side): no Explore list asks before then.
		until = r.Until
	} else if !h.down {
		log.Printf("explore: metadata server is not answering, Explore uses kept lists and tries again later: %v", err)
	}
	h.down, h.fails, h.until = true, h.fails+1, until
	return false
}

// emptyRetry is the wait before rebuilding a pool that came out with no
// seeds (the metadata server was slow or down): 15 s, 30 s, 60 s, then every
// minute until it fills.
const emptyTriesMax = 15

func emptyRetry(tries int) time.Duration {
	return min(15*time.Second<<min(tries, 2), time.Minute)
}

// noteEmpty records one pool's build: an empty pool is never final, so it is
// built again soon; a pool with seeds clears its retries.
func (e *Explore) noteEmpty(k PoolKey, empty bool, now time.Time) {
	e.mu.Lock()
	if !empty {
		delete(e.empty, k)
		e.mu.Unlock()
		return
	}
	if e.empty == nil {
		e.empty = map[PoolKey]int{}
	}
	tries := e.empty[k]
	e.empty[k] = tries + 1
	e.mu.Unlock()
	// A pool that stays empty for a quarter hour may truly have nothing; it
	// still refills when the server answers again, and with the daily build.
	if tries < emptyTriesMax {
		e.QueuePool(k, now.Add(emptyRetry(tries)), false)
	}
}

// refillEmpty rebuilds every empty pool now: the metadata server answers again.
func (e *Explore) refillEmpty() {
	e.mu.Lock()
	keys := make([]PoolKey, 0, len(e.empty))
	for k := range e.empty {
		keys = append(keys, k)
	}
	e.mu.Unlock()
	now := e.Now()
	for _, k := range keys {
		e.QueuePool(k, now, false)
	}
}

// serverBusy says err is the server asking HolyArrs to wait.
func serverBusy(err error) bool {
	return errors.Is(err, metadata.ErrRateLimited) || errors.Is(err, metadata.ErrServiceBusy)
}

// serverDown says err means the metadata server did not answer, or asked
// to wait with a Retry-After that is still running.
func serverDown(err error) bool {
	if serverBusy(err) {
		_, ok := metadata.CurrentRefusal(time.Now())
		return ok
	}
	return errors.Is(err, metadata.ErrServiceUnavailable)
}

// lookupsOff says the owner turned lookups off, so Explore asks no one.
func (e *Explore) lookupsOff() bool { return e.LookupsOff != nil && e.LookupsOff() }
