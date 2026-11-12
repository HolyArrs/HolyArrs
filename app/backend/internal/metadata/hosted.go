package metadata

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// DefaultServiceURL is the HolyArrs metadata server every install uses by
// default. It is the one place the address lives; EnvServiceURL overrides it.
const DefaultServiceURL = "https://api.holyarrs.com"

// OldServiceURL is the server's first address. It answers until version 2,
// and the server still writes picture addresses with it.
const OldServiceURL = "https://holyarrs-api.coderluii.dev"

// EnvServiceURL names the metadata server address override.
const EnvServiceURL = "HOLYARRS_METADATA_URL"

// EnvTMDBKey names a person's own TMDB read access token. Set, HolyArrs asks
// TMDB directly with it instead of the metadata server (not shown in the app).
const EnvTMDBKey = "HOLYARRS_TMDB_KEY"

// ServiceURL is the metadata server address: EnvServiceURL, else the default.
// EnvServiceURL=off is not an address: it picks Your own keys (Modes), and
// the default address stays only so kept answers can be found.
func ServiceURL() string {
	if v := strings.TrimRight(strings.TrimSpace(os.Getenv(EnvServiceURL)), "/"); v != "" && !strings.EqualFold(v, "off") {
		return v
	}
	return DefaultServiceURL
}

// TMDBPath is where the metadata server answers TMDB reads with TMDB's own
// JSON; a TMDB client with this base and an installation credential works
// unchanged.
const TMDBPath = "/v1/tmdb"

// Base is the server origin, without a trailing slash.
func (s *Service) Base() string { return s.base }

// Register asks the server for a new installation credential. The server
// limits how often an address may do this; it needs no credential. A
// registration that failed (refused, server error, timeout, unreachable) is
// not tried again straight away: the wait starts at regWaitMin and doubles to
// regWaitMax, or is the server's Retry-After when that is longer. Inside the
// wait the busy answer comes back without asking the server.
func (s *Service) Register(ctx context.Context) (string, error) {
	if s.regHold.held(time.Now()) {
		return "", ErrServiceBusy
	}
	tok, err := s.register(ctx)
	switch {
	case err == nil:
		s.regHold.mu.Lock()
		s.regHold.fails = 0
		s.regHold.mu.Unlock()
	case ctx.Err() == nil && !errors.Is(err, ErrSourceOff): // a caller that gave up, or lookups off, says nothing about the server
		s.regHold.backOff(time.Now())
	}
	return tok, err
}

// ResetRegisterWait ends the wait after failed registrations (a mode picked
// in Settings), so the next lookup asks the server straight away.
func (s *Service) ResetRegisterWait() {
	s.regHold.mu.Lock()
	s.regHold.until, s.regHold.fails = time.Time{}, 0
	s.regHold.mu.Unlock()
}

// regWaitMin and regWaitMax bound the wait after a failed registration.
const (
	regWaitMin = 30 * time.Second
	regWaitMax = 15 * time.Minute
)

// backOff holds for regWaitMin, doubled for each failure in a row, up to
// regWaitMax. A longer hold already set (Retry-After) stays.
func (h *serviceHold) backOff(now time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	d := regWaitMax
	if h.fails < 5 {
		d = regWaitMin << h.fails
	}
	h.fails++
	if u := now.Add(d); u.After(h.until) {
		h.until = u
	}
}

func (s *Service) register(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, s.base+"/v1/installations", http.NoBody)
	req.Header.Set("Accept", "application/json")
	resp, err := s.http.Do(req)
	if err != nil {
		return "", unreached(err)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	_ = resp.Body.Close()
	if err != nil {
		return "", ErrServiceUnavailable
	}
	switch {
	case resp.StatusCode == http.StatusCreated:
	case resp.StatusCode == http.StatusTooManyRequests:
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			s.regHold.set(ra, body, time.Now())
		}
		return "", ErrServiceBusy
	case resp.StatusCode >= 500 || resp.StatusCode == http.StatusRequestTimeout:
		return "", ErrServiceUnavailable
	default:
		return "", ErrServiceAccess
	}
	var b struct {
		SchemaVersion int    `json:"schema_version"`
		Token         string `json:"token"`
	}
	d := json.NewDecoder(bytes.NewReader(body))
	if d.Decode(&b) != nil || b.SchemaVersion != 1 || len(b.Token) < 20 || len(b.Token) > 512 || strings.ContainsAny(b.Token, " \t\r\n") {
		return "", ErrServiceInvalid
	}
	return b.Token, nil
}

// Health answers whether the metadata server is up, asking its cheap
// /readyz at most once per TTL (default a minute).
type Health struct {
	Service *Service
	TTL     time.Duration

	mu      sync.Mutex
	checked time.Time
	up      bool
}

// Up reports the cached answer, asking the server again when it is old.
func (h *Health) Up(ctx context.Context) bool {
	ttl := h.TTL
	if ttl <= 0 {
		ttl = time.Minute
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.checked.IsZero() && time.Since(h.checked) < ttl {
		return h.up
	}
	h.up = h.ask(ctx) == nil
	h.checked = time.Now()
	return h.up
}

func (h *Health) ask(ctx context.Context) error {
	if h.Service == nil {
		return errors.New("no metadata server")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, h.Service.base+"/readyz", nil)
	resp, err := h.Service.http.Do(req)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New("metadata server not ready")
	}
	return nil
}
