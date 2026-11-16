package metadata

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Refusal is the server's latest "wait" (a 429 or 503) with its reason
// code (registration_limit, client_limit, registry_full, upstream_busy,
// ...) and when asking again is welcome.
type Refusal struct {
	Code  string
	Until time.Time
}

var refusal struct {
	mu sync.Mutex
	r  Refusal
}

// ResetRefusal forgets the latest refusal (tests, a mode picked).
func ResetRefusal() {
	refusal.mu.Lock()
	refusal.r = Refusal{}
	refusal.mu.Unlock()
}

// CurrentRefusal is the refusal still running at now.
func CurrentRefusal(now time.Time) (Refusal, bool) {
	refusal.mu.Lock()
	defer refusal.mu.Unlock()
	return refusal.r, now.Before(refusal.r.Until)
}

// refusalBody reads the server's problem body: its code and its wait.
func refusalBody(body []byte) (code string, retry int) {
	var b struct {
		Error struct {
			Code  string `json:"code"`
			Retry int    `json:"retry_after_seconds"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &b) != nil {
		return "", 0
	}
	return b.Error.Code, b.Error.Retry
}

// refusalWait reads Retry-After (seconds or a date), else the body's
// retry_after_seconds, else a minute; at most holdMax.
func refusalWait(value string, retry int, now time.Time) time.Duration {
	d := time.Minute
	if n, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && n >= 0 {
		d = time.Duration(n) * time.Second
	} else if at, err := http.ParseTime(value); err == nil && at.After(now) {
		d = at.Sub(now)
	} else if retry > 0 {
		d = time.Duration(retry) * time.Second
	}
	return min(d, holdMax)
}

// noteRefusal records a 429 or 503 from the server. A new hold is logged
// once with its reason; refusals inside a running hold only lengthen it.
func noteRefusal(status int, retryAfter string, body []byte, now time.Time) time.Duration {
	code, retry := refusalBody(body)
	if code == "" {
		code = "upstream_busy"
		if status == http.StatusTooManyRequests {
			code = "client_limit"
		}
	}
	d := refusalWait(retryAfter, retry, now)
	if d <= 0 {
		return d
	}
	refusal.mu.Lock()
	running := now.Before(refusal.r.Until)
	if u := now.Add(d); !running || u.After(refusal.r.Until) {
		refusal.r.Until = u
		refusal.r.Code = code
	}
	until := refusal.r.Until
	refusal.mu.Unlock()
	if !running {
		log.Printf("metadata: the server asked HolyArrs to wait (%s); background lookups pause until %s", code, until.Format(time.RFC3339))
	}
	return d
}

// peekRefusal reads a refusal's small body and puts it back for the caller.
func peekRefusal(resp *http.Response) []byte {
	if resp.Body == nil {
		return nil
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	resp.Body = struct {
		io.Reader
		io.Closer
	}{io.MultiReader(bytes.NewReader(b), resp.Body), resp.Body}
	return b
}

// BusyLine is the message for a busy answer while the server asked to wait,
// and the seconds left (0 when no refusal runs).
func BusyLine(now time.Time) (string, int) {
	r, ok := CurrentRefusal(now)
	if !ok {
		return ErrServiceBusy.Error(), 0
	}
	secs := int((r.Until.Sub(now) + time.Second - 1) / time.Second)
	// Each reason in plain words, with what the owner can do.
	line := map[string]string{
		"registration_limit": "The metadata server is not signing up more installs from this address right now. HolyArrs asks again on its own; keep the /config folder between restarts so this install keeps its sign-up.",
		"registry_full":      "The metadata server has no room for new installs right now. HolyArrs asks again on its own within the hour.",
		"client_limit":       "This address sent the metadata server too many requests. HolyArrs waits and asks again on its own.",
		"installation_limit": "This install sent the metadata server too many requests. HolyArrs waits and asks again on its own.",
		"upstream_busy":      "The metadata server is busy right now. HolyArrs asks again on its own in a moment.",
	}[busyCode(r.Code)]
	return line, secs
}

// busyCode is the reason a busy line is chosen by; others read as busy.
func busyCode(code string) string {
	switch code {
	case "registration_limit", "registry_full", "client_limit", "installation_limit":
		return code
	}
	return "upstream_busy"
}

// SetRefusal records a refusal learned elsewhere (tests).
func SetRefusal(r Refusal) {
	refusal.mu.Lock()
	refusal.r = r
	refusal.mu.Unlock()
}
