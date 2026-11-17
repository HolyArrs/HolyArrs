package outbound

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"
)

// fakeIPKeep is how long a fake-address refusal is still shown after the
// last time it happened.
const fakeIPKeep = 30 * time.Minute

// DialFunc is the shape of a transport's DialContext.
type DialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// FakeIPError is a dial refused because the name resolved into 198.18.0.0/15,
// the range Clash and Surge hand out in fake-IP mode. The range stays
// refused; the error only names the cause.
type FakeIPError struct{ Host string }

func (e *FakeIPError) Error() string   { return FakeIPMessage(e.Host) }
func (e *FakeIPError) Is(t error) bool { return t == ErrAddressRefused }

// FakeIPMessage is the plain-words line for a fake-address refusal.
func FakeIPMessage(host string) string {
	return "Your router or VPN is sending HolyArrs to a fake address (198.18.x.x). This usually comes from Clash or Surge. Add " + host + " to its direct list."
}

// errFakeIPAddr is what the policy dialer returns for an address in the
// fake range; WatchFakeIP turns it into a FakeIPError with the host name.
var errFakeIPAddr error = fakeIPAddr{}

type fakeIPAddr struct{}

func (fakeIPAddr) Error() string   { return "address in 198.18.0.0/15 refused by the network policy" }
func (fakeIPAddr) Is(t error) bool { return t == ErrAddressRefused }

var fakeRange = func() *net.IPNet { _, n, _ := net.ParseCIDR("198.18.0.0/15"); return n }()

// IsFakeIP says ip is in 198.18.0.0/15.
func IsFakeIP(ip net.IP) bool { return ip != nil && fakeRange.Contains(ip) }

// fakeSeen keeps when each host was last refused for a fake address.
var fakeSeen = struct {
	mu    sync.Mutex
	hosts map[string]time.Time
}{hosts: map[string]time.Time{}}

// WatchFakeIP wraps dial so a refusal for a fake address comes back as a
// FakeIPError naming the host, and is remembered for FakeIPHost.
func WatchFakeIP(dial DialFunc) DialFunc {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			host = addr
		}
		c, err := dial(ctx, network, addr)
		if err != nil {
			if errors.Is(err, errFakeIPAddr) {
				noteFakeIP(host, time.Now())
				return nil, &FakeIPError{Host: host}
			}
			return c, err
		}
		clearFakeIP(host)
		return c, nil
	}
}

// FakeIPHost is the host most lately refused for a fake address in the past
// half hour, or "" when none.
func FakeIPHost() string {
	fakeSeen.mu.Lock()
	defer fakeSeen.mu.Unlock()
	var host string
	var last time.Time
	for h, at := range fakeSeen.hosts {
		if time.Since(at) > fakeIPKeep {
			delete(fakeSeen.hosts, h)
		} else if at.After(last) {
			host, last = h, at
		}
	}
	return host
}

// FakeIPSeen says host was refused for a fake address in the past half hour
// and has not connected since.
func FakeIPSeen(host string) bool {
	fakeSeen.mu.Lock()
	defer fakeSeen.mu.Unlock()
	at, ok := fakeSeen.hosts[host]
	return ok && time.Since(at) <= fakeIPKeep
}

func noteFakeIP(host string, at time.Time) {
	fakeSeen.mu.Lock()
	fakeSeen.hosts[host] = at
	fakeSeen.mu.Unlock()
}

func clearFakeIP(host string) {
	fakeSeen.mu.Lock()
	delete(fakeSeen.hosts, host)
	fakeSeen.mu.Unlock()
}
