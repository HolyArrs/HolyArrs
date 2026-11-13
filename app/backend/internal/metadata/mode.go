package metadata

import (
	"errors"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
)

// Mode says where details come from: the HolyArrs metadata server (the
// default), the owner's own keys (the server is never contacted), or off
// (no lookups at all; Library, downloads and requests keep working).
type Mode string

const (
	ModeServer Mode = "full"
	ModeOwn    Mode = "own"
	ModeOff    Mode = "off"
)

// EnvMode names the Docker setting for the mode: full, own or off. Any other
// value turns lookups off and Health names the value.
const EnvMode = "HOLYARRS_METADATA_MODE"

// ModeGroup and ModeKey are the settings row for the mode picked in
// Settings. A missing row means the metadata server.
const ModeGroup, ModeKey = "this_app", "metadata_mode"

// ErrSourceOff means the lookup is off on this install (the mode keeps it
// from going out). It is never sent anywhere.
var ErrSourceOff = errors.New("this lookup is off on this install")

// ErrModeLocked means the Docker settings set the mode, so Settings cannot.
var ErrModeLocked = errors.New("set by your Docker settings")

// ParseMode reads a mode loosely (case, spaces and a few other spellings).
// ok is false for anything it does not know.
func ParseMode(raw string) (Mode, bool) {
	v := strings.ToLower(strings.TrimSpace(raw))
	v = strings.NewReplacer("-", "", "_", "", " ", "").Replace(v)
	switch v {
	case "full", "server", "holyarrs", "holyarrsserver", "on", "default":
		return ModeServer, true
	case "own", "ownkeys", "keys", "yourownkeys", "mykeys":
		return ModeOwn, true
	case "off", "none", "disabled", "false", "0":
		return ModeOff, true
	}
	return ModeOff, false
}

// Modes holds the current mode. The Docker setting, when there is one, wins
// and locks the picker; otherwise the stored setting applies, and a change
// applies at once (no restart).
type Modes struct {
	env    Mode
	envSet bool
	bad    string
	cur    atomic.Value // Mode
	// OnSet runs after a mode is picked in Settings; set once at start.
	OnSet func()
}

// NewModes reads the Docker settings with getenv: EnvMode, else
// HOLYARRS_METADATA_URL=off (which means Your own keys).
func NewModes(getenv func(string) string) *Modes {
	m := &Modes{}
	if raw := strings.TrimSpace(getenv(EnvMode)); raw != "" {
		mode, ok := ParseMode(raw)
		m.env, m.envSet = mode, true
		if !ok {
			m.bad = raw
		}
	} else if strings.EqualFold(strings.TrimSpace(getenv(EnvServiceURL)), "off") {
		m.env, m.envSet = ModeOwn, true
	}
	m.cur.Store(ModeServer)
	return m
}

// EnvModes reads the Docker settings from the environment.
func EnvModes() *Modes { return NewModes(os.Getenv) }

// Current is the mode in force. A nil *Modes is the metadata server.
func (m *Modes) Current() Mode {
	if m == nil {
		return ModeServer
	}
	if m.envSet {
		return m.env
	}
	if v, ok := m.cur.Load().(Mode); ok {
		return v
	}
	return ModeServer
}

// Locked says the Docker settings set the mode.
func (m *Modes) Locked() bool { return m != nil && m.envSet }

// BadValue is the Docker value that was not understood ("" when none).
func (m *Modes) BadValue() string {
	if m == nil {
		return ""
	}
	return m.bad
}

// Set applies the mode picked in Settings. It is refused while the Docker
// settings set it.
func (m *Modes) Set(v Mode) error {
	if m.Locked() {
		return ErrModeLocked
	}
	if _, ok := ParseMode(string(v)); !ok {
		return errors.New("mode must be full, own or off")
	}
	m.cur.Store(v)
	if m.OnSet != nil {
		m.OnSet()
	}
	return nil
}

// Server says requests may go to the metadata server.
func (m *Modes) Server() bool { return m.Current() == ModeServer }

// Direct says this install may ask the outside sources itself (Your own keys).
func (m *Modes) Direct() bool { return m.Current() == ModeOwn }

// Guard wraps a client to an outside source: its requests go out only while
// allow says so for the current mode, and answer ErrSourceOff otherwise
// without being sent.
func (m *Modes) Guard(next http.RoundTripper, allow func(Mode) bool) http.RoundTripper {
	if next == nil {
		next = http.DefaultTransport
	}
	return modeGuard{m: m, next: next, allow: allow}
}

// OwnOnly allows Your own keys only.
func OwnOnly(v Mode) bool { return v == ModeOwn }

// NotOff allows every mode but off.
func NotOff(v Mode) bool { return v != ModeOff }

type modeGuard struct {
	m     *Modes
	next  http.RoundTripper
	allow func(Mode) bool
}

func (g modeGuard) RoundTrip(req *http.Request) (*http.Response, error) {
	if g.allow != nil && !g.allow(g.m.Current()) {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, ErrSourceOff
	}
	return g.next.RoundTrip(req)
}

// unreached is the error for a request to the metadata server that did not
// get an answer: ErrSourceOff when the mode kept it in, ErrServiceSlow when
// it ran out of time, else ErrServiceUnavailable.
func unreached(err error) error {
	if errors.Is(err, ErrSourceOff) {
		return ErrSourceOff
	}
	var te interface{ Timeout() bool }
	if errors.As(err, &te) && te.Timeout() {
		return ErrServiceSlow
	}
	return ErrServiceUnavailable
}

// accessErr is the error for a credential that could not be read:
// ErrSourceOff when the mode kept the sign-up in, else ErrServiceAccess.
func accessErr(err error) error {
	if errors.Is(err, ErrSourceOff) {
		return ErrSourceOff
	}
	return ErrServiceAccess
}

// SourceOff says err comes from a lookup that is off on this install.
func SourceOff(err error) bool { return errors.Is(err, ErrSourceOff) }
