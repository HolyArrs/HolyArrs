package ai

import (
	"strings"
	"sync"
	"time"
)

// AI limits per key: reads a minute, actions an hour, and how long a
// confirm token lives.
const (
	ReadsPerMin  = 60
	ActsPerHour  = 5
	confirmFor   = 5 * time.Minute
	TextMax      = 300
	listMax      = 25
	bodyMax      = 64 << 10
	clientsMax   = 1000
	tokensMax    = 1000
	argsLogLimit = 500
)

// Limits is what AI connections keep in memory: use counts per key, the
// confirm tokens waiting for their second call and the client names.
type Limits struct {
	mu      sync.Mutex
	Reads   map[int64][]time.Time
	Acts    map[int64][]time.Time
	tokens  map[string]token
	clients map[int64]string
	now     func() time.Time
}

// token is one confirm step waiting for its second call.
type token struct {
	user, key int64
	tool      string
	args      string
	until     time.Time
}

func (s *Limits) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

// Allow counts one use against the limit in the window, and when full
// says how long until the oldest use leaves it.
func (s *Limits) Allow(m *map[int64][]time.Time, key int64, limit int, window time.Duration) (bool, time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if *m == nil {
		*m = map[int64][]time.Time{}
	}
	now := s.clock()
	keep := (*m)[key][:0]
	for _, t := range (*m)[key] {
		if now.Sub(t) < window {
			keep = append(keep, t)
		}
	}
	if len(keep) >= limit {
		(*m)[key] = keep
		return false, window - now.Sub(keep[0])
	}
	(*m)[key] = append(keep, now)
	return true, 0
}

// rememberClient keeps the client's name per key for History.
func (h *Handlers) rememberClient(key int64, name string) {
	name = clean(name, 60)
	if name == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients == nil || len(h.clients) > clientsMax {
		h.clients = map[int64]string{}
	}
	if _, known := h.clients[key]; !known || !strings.Contains(name, "/") {
		h.clients[key] = name
	}
}

func (h *Handlers) clientOf(key int64) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if n := h.clients[key]; n != "" {
		return n
	}
	return "an AI app"
}

// clientFromAgent is a fallback client name from the User-Agent.
func clientFromAgent(ua string) string {
	name, _, _ := strings.Cut(strings.TrimSpace(ua), " ")
	return name
}
