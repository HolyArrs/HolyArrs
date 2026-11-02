// Package extras serves keyed extra sources: fanart.tv artwork (cached in
// memory) and MDBList ratings (stored, see mdblist.go). Each source is off
// unless its key is set; key values are never logged, printed or returned.
package extras

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	FanartURL  = "https://webservice.fanart.tv/v3"
	MDBListURL = "https://api.mdblist.com"
	// States reported in /v1/stats.
	StateOK            = "ok"
	StateDisabledNoKey = "disabled_no_key"
)

var (
	// ErrDisabled is returned when the source has no key.
	ErrDisabled = errors.New("extras: disabled_no_key")
	// ErrNotFound is returned when the source knows nothing for the id.
	ErrNotFound = errors.New("extras: not_found")
)

// Status is one source's state for /v1/stats.
type Status struct {
	State     string `json:"state"`
	Cached    int    `json:"cached"`
	Failures  int64  `json:"failures"`
	LastError string `json:"last_error,omitempty"`
}

// cache is a small in-memory cache with single flight per key.
type cache struct {
	TTL, MissTTL time.Duration
	Max          int
	Now          func() time.Time

	mu       sync.Mutex
	entries  map[string]entry
	flights  map[string]*flight
	failures int64
	lastErr  string
}

type entry struct {
	body []byte
	err  error
	at   time.Time
}

type flight struct {
	done chan struct{}
	e    entry
}

func (c *cache) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// errorTTL is how long an upstream failure is kept before a new ask.
const errorTTL = 15 * time.Minute

func (c *cache) ttl(e entry) time.Duration {
	if e.err != nil && !errors.Is(e.err, ErrNotFound) {
		// An upstream failure is re-asked soon; only a real "none" is kept long.
		return errorTTL
	}
	if e.err != nil {
		if c.MissTTL > 0 {
			return c.MissTTL
		}
		return time.Hour
	}
	return c.TTL
}

// get answers key from cache or load; cache is "hit", "miss" or "stale".
func (c *cache) get(ctx context.Context, key string, load func(context.Context) ([]byte, error)) ([]byte, string, error) {
	c.mu.Lock()
	if c.entries == nil {
		c.entries, c.flights = map[string]entry{}, map[string]*flight{}
	}
	old, had := c.entries[key]
	if had && c.now().Sub(old.at) < c.ttl(old) {
		c.mu.Unlock()
		return old.body, "hit", old.err
	}
	if f, ok := c.flights[key]; ok {
		c.mu.Unlock()
		select {
		case <-f.done:
			return f.e.body, "hit", f.e.err
		case <-ctx.Done():
			return nil, "", ctx.Err()
		}
	}
	f := &flight{done: make(chan struct{})}
	c.flights[key] = f
	c.mu.Unlock()

	body, err := load(ctx)
	f.e = entry{body: body, err: err, at: c.now()}
	failed := err != nil && !errors.Is(err, ErrNotFound)
	c.mu.Lock()
	delete(c.flights, key)
	if failed {
		c.failures++
		c.lastErr = "upstream_failed"
	}
	// A failed refetch keeps serving the last good answer for a while.
	if failed && had && old.err == nil {
		kept := old
		kept.at = c.now().Add(c.ttl(f.e) - c.ttl(old))
		c.entries[key] = kept
		f.e = kept
		c.mu.Unlock()
		close(f.done)
		return old.body, "stale", nil
	}
	if ctx.Err() == nil {
		if _, has := c.entries[key]; !has && len(c.entries) >= max(c.Max, 1) {
			c.evict()
		}
		c.entries[key] = f.e
	}
	c.mu.Unlock()
	close(f.done)
	return body, "miss", err
}

func (c *cache) evict() {
	var oldest string
	var at time.Time
	for k, e := range c.entries {
		if oldest == "" || e.at.Before(at) {
			oldest, at = k, e.at
		}
	}
	delete(c.entries, oldest)
}

func (c *cache) status(enabled bool) Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := Status{State: StateOK, Cached: len(c.entries), Failures: c.failures, LastError: c.lastErr}
	if !enabled {
		s = Status{State: StateDisabledNoKey}
	}
	return s
}

// getJSON reads one upstream JSON answer (max 2 MiB). 404 is ErrNotFound.
// The URL may carry a key, so errors never include it.
func getJSON(ctx context.Context, client *http.Client, raw, userAgent string, header http.Header, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return errors.New("extras: bad request")
	}
	for k, v := range header {
		req.Header[k] = v
	}
	req.Header.Set("Accept", "application/json")
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("extras: upstream unreachable")
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return ErrNotFound
	}
	if resp.StatusCode != 200 {
		return errors.New("extras: upstream status " + http.StatusText(resp.StatusCode))
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(out); err != nil {
		return errors.New("extras: bad upstream answer")
	}
	return nil
}
