package outbound

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

func TestFakeIPRange(t *testing.T) {
	for ip, want := range map[string]bool{"198.18.0.1": true, "198.19.255.254": true, "198.20.0.1": false, "8.8.8.8": false, "::1": false} {
		if got := IsFakeIP(net.ParseIP(ip)); got != want {
			t.Errorf("IsFakeIP(%s) = %v, want %v", ip, got, want)
		}
	}
}

func TestFakeIPStaysBlocked(t *testing.T) {
	if PublicOnly(net.ParseIP("198.18.3.4")) {
		t.Fatal("198.18.0.0/15 must stay refused")
	}
}

func TestWatchFakeIPNamesTheHost(t *testing.T) {
	resetFakeIP()
	refuse := func(context.Context, string, string) (net.Conn, error) {
		return nil, &net.OpError{Op: "dial", Err: errFakeIPAddr}
	}
	_, err := WatchFakeIP(refuse)(context.Background(), "tcp", "api.holyarrs.com:443")
	var fe *FakeIPError
	if !errors.As(err, &fe) || fe.Host != "api.holyarrs.com" {
		t.Fatalf("err = %v, want FakeIPError for api.holyarrs.com", err)
	}
	if !errors.Is(err, ErrAddressRefused) {
		t.Fatal("a fake address is still a refused address")
	}
	if !strings.Contains(err.Error(), "198.18.x.x") || !strings.Contains(err.Error(), "api.holyarrs.com") {
		t.Fatalf("message = %q", err.Error())
	}
	if FakeIPHost() != "api.holyarrs.com" {
		t.Fatalf("FakeIPHost = %q", FakeIPHost())
	}
	// A later good dial to the same host clears it.
	ok := func(context.Context, string, string) (net.Conn, error) { c, _ := net.Pipe(); return c, nil }
	c, err := WatchFakeIP(ok)(context.Background(), "tcp", "api.holyarrs.com:443")
	if err != nil {
		t.Fatal(err)
	}
	_ = c.Close()
	if FakeIPHost() != "" {
		t.Fatalf("FakeIPHost after a good dial = %q", FakeIPHost())
	}
}

func TestFakeIPHostExpires(t *testing.T) {
	resetFakeIP()
	noteFakeIP("x.example", time.Now().Add(-fakeIPKeep-time.Second))
	if FakeIPHost() != "" {
		t.Fatal("an old sighting should not show")
	}
}

func TestPolicyDialerMarksFakeIP(t *testing.T) {
	d := PolicyDialer(PublicOnly)
	err := d.Control("tcp4", "198.18.0.5:443", nil)
	if !errors.Is(err, errFakeIPAddr) || !errors.Is(err, ErrAddressRefused) {
		t.Fatalf("err = %v", err)
	}
	if err := d.Control("tcp4", "10.0.0.1:443", nil); errors.Is(err, errFakeIPAddr) || !errors.Is(err, ErrAddressRefused) {
		t.Fatalf("private err = %v", err)
	}
}

func resetFakeIP() {
	fakeSeen.mu.Lock()
	clear(fakeSeen.hosts)
	fakeSeen.mu.Unlock()
}

func TestFakeIPPerHost(t *testing.T) {
	resetFakeIP()
	noteFakeIP("a.example", time.Now().Add(-time.Minute))
	noteFakeIP("b.example", time.Now())
	if FakeIPHost() != "b.example" || !FakeIPSeen("a.example") {
		t.Fatalf("host = %q", FakeIPHost())
	}
	clearFakeIP("b.example")
	if FakeIPHost() != "a.example" || FakeIPSeen("b.example") {
		t.Fatalf("after b connects: %q", FakeIPHost())
	}
}
