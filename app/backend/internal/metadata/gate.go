package metadata

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Gate is the one door every request to the metadata server goes through
// (text answers and pictures). It keeps the install well inside the server's
// limit for one credential (480 a minute, a burst of 300):
//
//   - every request takes a token from one bucket (gateRate a second, a
//     burst of gateBurst); a person's request waits for one;
//   - background requests (marked Ahead: warmers, fetch-ahead, preloads,
//     refreshers, repairs) also take one from their own small bucket
//     (gateAheadRate a second) and only while gateReserve tokens stay for
//     people, so a person's page always goes first;
//   - a 429 or 503 pauses background requests for its Retry-After (at least
//     gateAheadPause): they are answered here with a 429 and never sent. A
//     429 also holds people's requests for its Retry-After (at most
//     gatePersonHoldMax), so nothing is sent into a refusal.
type Gate struct {
	Next http.RoundTripper
	// Now is the clock (tests).
	Now func() time.Time

	mu         sync.Mutex
	tokens     float64
	ahead      float64
	last       time.Time
	aheadUntil time.Time
	personHold time.Time
	started    bool

	countOnce sync.Once
	cmu       sync.Mutex
	counts    map[string]int
	status    map[string]int

	// countryOff: the owner switched "Count this install" off (Settings,
	// This app); every request then carries X-Country-Count: off.
	countryOff atomic.Bool
}

// CountryCountHeader tells the metadata server this install is not counted
// in its per-country install total. It is sent, as "off", only when the
// owner switched the count off.
const CountryCountHeader = "X-Country-Count"

// SetCountryCount sets "Count this install" (on by default).
func (g *Gate) SetCountryCount(on bool) { g.countryOff.Store(!on) }

const (
	gateRate          = 5.0  // a second, for every request (300 a minute)
	gateBurst         = 60.0 // well under the server's 300
	gateAheadRate     = 0.5  // a second, for background requests (30 a minute)
	gateAheadBurst    = 3.0
	gateReserve       = 20.0 // tokens background requests leave for people
	gateAheadPause    = 30 * time.Second
	gateAheadPauseMax = 10 * time.Minute
	gatePersonHoldMax = 10 * time.Second
)

// aheadPaused is the latest background pause of any Gate (unix ns).
var aheadPaused atomic.Int64

// WaitAhead waits out the background pause, if any; false when ctx ended.
func WaitAhead(ctx context.Context) bool {
	for {
		d := time.Until(time.Unix(0, aheadPaused.Load()))
		if d <= 0 {
			return ctx.Err() == nil
		}
		t := time.NewTimer(d)
		select {
		case <-ctx.Done():
			t.Stop()
			return false
		case <-t.C:
		}
	}
}

// GateHeld marks a 429 answered by the Gate itself (never sent).
const GateHeld = "X-Holyarrs-Gate"

func (g *Gate) now() time.Time {
	if g.Now != nil {
		return g.Now()
	}
	return time.Now()
}

func gateAhead(req *http.Request) bool {
	return req.Header.Get("X-Fetch") == "ahead" || IsAhead(req.Context())
}

// refill adds the tokens earned since the last call; g.mu is held.
func (g *Gate) refill(now time.Time) {
	if !g.started {
		g.tokens, g.ahead, g.last, g.started = gateBurst, gateAheadBurst, now, true
		return
	}
	dt := now.Sub(g.last).Seconds()
	if dt <= 0 {
		return
	}
	g.last = now
	g.tokens = min(gateBurst, g.tokens+dt*gateRate)
	g.ahead = min(gateAheadBurst, g.ahead+dt*gateAheadRate)
}

// take waits for this request's turn. held answers a background request
// while the server asked us to wait: it is not sent.
func (g *Gate) take(ctx context.Context, ahead bool) (held time.Duration, err error) {
	for {
		g.mu.Lock()
		now := g.now()
		g.refill(now)
		var wait time.Duration
		switch {
		case ahead && now.Before(g.aheadUntil):
			d := g.aheadUntil.Sub(now)
			g.mu.Unlock()
			return d, nil
		case !ahead && now.Before(g.personHold):
			wait = g.personHold.Sub(now)
		case ahead:
			if g.tokens >= gateReserve+1 && g.ahead >= 1 {
				g.tokens--
				g.ahead--
				g.mu.Unlock()
				return 0, nil
			}
			need := max((gateReserve+1-g.tokens)/gateRate, (1-g.ahead)/gateAheadRate)
			wait = time.Duration(need * float64(time.Second))
		default:
			if g.tokens >= 1 {
				g.tokens--
				g.mu.Unlock()
				return 0, nil
			}
			wait = time.Duration((1 - g.tokens) / gateRate * float64(time.Second))
		}
		g.mu.Unlock()
		wait = max(wait, 10*time.Millisecond)
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return 0, ctx.Err()
		case <-t.C:
		}
	}
}

// refused records a 429 or 503 from the server.
func (g *Gate) refused(code int, retryAfter string) {
	d := time.Duration(0)
	if n, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && n > 0 {
		d = time.Duration(n) * time.Second
	} else if at, err := http.ParseTime(retryAfter); err == nil {
		d = time.Until(at)
	}
	now := g.now()
	g.mu.Lock()
	defer g.mu.Unlock()
	pause := min(max(d, gateAheadPause), gateAheadPauseMax)
	if u := now.Add(pause); u.After(g.aheadUntil) {
		g.aheadUntil = u
		aheadPaused.Store(u.UnixNano())
	}
	if code == http.StatusTooManyRequests {
		hold := min(max(d, time.Second), gatePersonHoldMax)
		if u := now.Add(hold); u.After(g.personHold) {
			g.personHold = u
		}
		// The server's bucket is empty: ours starts over from empty too.
		g.tokens = 0
	}
}

func (g *Gate) RoundTrip(req *http.Request) (*http.Response, error) {
	ahead := gateAhead(req)
	t0 := time.Now()
	held, err := g.take(req.Context(), ahead)
	t1 := time.Now()
	if err != nil {
		return nil, err
	}
	if held > 0 {
		g.count(req, ahead, "held")
		secs := int(held/time.Second) + 1
		h := http.Header{}
		h.Set("Retry-After", strconv.Itoa(secs))
		h.Set("Content-Type", "application/json")
		h.Set(GateHeld, "held")
		body := fmt.Sprintf(`{"error":{"code":"client_held","retry_after_seconds":%d}}`, secs)
		return &http.Response{Status: "429 Too Many Requests", StatusCode: http.StatusTooManyRequests, Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1,
			Header: h, Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body)), Request: req}, nil
	}
	if g.countryOff.Load() {
		req = req.Clone(req.Context())
		req.Header.Set(CountryCountHeader, "off")
	}
	resp, err := g.Next.RoundTrip(req)
	if err != nil {
		g.count(req, ahead, "error")
		return resp, err
	}
	g.count(req, ahead, strconv.Itoa(resp.StatusCode))
	if os.Getenv(EnvCountRequests) != "" && time.Since(t0) > 150*time.Millisecond {
		log.Printf("metaslow: %s wait=%v headers=%v", req.URL.Path, t1.Sub(t0).Round(time.Millisecond), time.Since(t1).Round(time.Millisecond))
	}
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
		g.refused(resp.StatusCode, resp.Header.Get("Retry-After"))
	}
	return resp, nil
}

// EnvCountRequests turns on a debug-only counter of the requests to the
// metadata server, by lane, path and caller, written to the log every 30 s.
const EnvCountRequests = "HOLYARRS_METADATA_COUNT"

var countIDs = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F-]{27}|\b\d{2,}\b|[0-9a-f]{16,}`)

func (g *Gate) count(req *http.Request, ahead bool, status string) {
	if os.Getenv(EnvCountRequests) == "" {
		return
	}
	g.countOnce.Do(func() {
		g.counts, g.status = map[string]int{}, map[string]int{}
		go func() {
			for range time.Tick(30 * time.Second) {
				g.dump()
			}
		}()
	})
	lane := "person"
	if ahead {
		lane = "ahead"
	}
	key := lane + " " + status + " " + countIDs.ReplaceAllString(req.URL.Path, ":id") + " " + gateCaller()
	if status == "held" {
		key = "HELD " + key
	}
	g.cmu.Lock()
	g.counts[key]++
	g.status[status]++
	g.cmu.Unlock()
}

func (g *Gate) dump() {
	g.cmu.Lock()
	defer g.cmu.Unlock()
	keys := make([]string, 0, len(g.counts))
	sent := 0
	for k, n := range g.counts {
		keys = append(keys, k)
		if !strings.HasPrefix(k, "HELD ") {
			sent += n
		}
	}
	sort.Slice(keys, func(i, j int) bool { return g.counts[keys[i]] > g.counts[keys[j]] })
	var b strings.Builder
	fmt.Fprintf(&b, "metacount: sent=%d status=%v", sent, g.status)
	for _, k := range keys {
		fmt.Fprintf(&b, "\nmetacount: %6d %s", g.counts[k], k)
	}
	log.Print(b.String())
}

// gateCaller names the first frames outside net/http and the Gate.
func gateCaller() string {
	pc := make([]uintptr, 64)
	n := runtime.Callers(4, pc)
	fr := runtime.CallersFrames(pc[:n])
	var out []string
	for {
		f, more := fr.Next()
		fn := f.Function
		if !strings.HasPrefix(fn, "net/http") && !strings.HasPrefix(fn, "runtime") && !strings.Contains(fn, "metadata.(*Gate)") && !strings.Contains(fn, "metadata.HostRoute") &&
			!strings.Contains(fn, "ArtworkCache).get") && !strings.Contains(fn, "ArtworkCache).download") && !strings.Contains(fn, "ArtworkCache).fetch") {
			if i := strings.LastIndex(fn, "/"); i >= 0 {
				fn = fn[i+1:]
			}
			out = append(out, fn)
			if len(out) == 6 {
				break
			}
		}
		if !more {
			break
		}
	}
	return strings.Join(out, "<")
}
