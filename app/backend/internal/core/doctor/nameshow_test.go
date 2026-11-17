package doctor

import (
	"context"
	"net"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

func fakeDial(err error, calls *int) Dial {
	return func(context.Context, string, string) (net.Conn, error) {
		*calls++
		return nil, err
	}
}

func TestNameLineShowsWhatTheNameTurnedInto(t *testing.T) {
	ip := &net.TCPAddr{IP: net.ParseIP("172.18.0.5"), Port: 8080}
	cases := []struct {
		name  string
		c     Cause
		url   string
		err   error
		want  string
		dials int
	}{
		{"not found", DNS, "http://qbittorrent:8080", &net.DNSError{Name: "qbittorrent", IsNotFound: true},
			"qbittorrent → not found. HolyArrs can't find this name. Use the IP instead.", 1},
		{"refused", Refused, "http://qbittorrent:8080", &net.OpError{Op: "dial", Net: "tcp", Addr: ip, Err: syscall.ECONNREFUSED},
			"qbittorrent → 172.18.0.5 → refused on port 8080. The name works, but nothing answers on that port. Check the port.", 1},
		{"timeout", Timeout, "http://sab:8080/sabnzbd", &net.OpError{Op: "dial", Net: "tcp", Addr: ip, Err: context.DeadlineExceeded},
			"sab → 172.18.0.5 → no answer on port 8080. The name works, but nothing answers in time. Check the port, and that no firewall is in the way.", 1},
		{"ip skips the name step", Refused, "http://172.18.0.5:8080", &net.OpError{Op: "dial", Net: "tcp", Addr: ip, Err: syscall.ECONNREFUSED}, "", 0},
		{"ipv6 skips the name step", Refused, "http://[fd00::5]:8080", nil, "", 0},
		{"other causes keep their line", Auth, "http://qbittorrent:8080", nil, "", 0},
		{"network rule refuses", Refused, "http://nas:8080", outbound.ErrAddressRefused, "", 1},
		{"public name not found is a typo", DNS, "https://indexr.example.com/api", &net.DNSError{Name: "indexr.example.com", IsNotFound: true}, "", 0},
		{"home name not found", DNS, "http://nas.lan:5000", &net.DNSError{Name: "nas.lan", IsNotFound: true},
			"nas.lan → not found. HolyArrs can't find this name. Use the IP instead.", 1},
		{"resolver down is no answer", DNS, "http://nas:8080", &net.DNSError{Name: "nas", IsTemporary: true}, "", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := 0
			if got := NameLine(tc.c, tc.url, fakeDial(tc.err, &n)); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
			if n != tc.dials {
				t.Fatalf("dialled %d times, want %d", n, tc.dials)
			}
		})
	}
}

func TestNameLineNeedsTheCallersDial(t *testing.T) {
	// No dial means a proxy carries the calls: the name is never looked up here.
	if got := NameLine(DNS, "http://qbittorrent:8080", nil); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestNameLineDefaultPort(t *testing.T) {
	var addr string
	dial := func(_ context.Context, _, a string) (net.Conn, error) {
		addr = a
		return nil, &net.OpError{Op: "dial", Net: "tcp", Addr: &net.TCPAddr{IP: net.ParseIP("10.0.0.2"), Port: 443}, Err: syscall.ECONNREFUSED}
	}
	got := NameLine(Refused, "https://jelly.lan/web", dial)
	if addr != "jelly.lan:443" || !strings.Contains(got, "10.0.0.2 → refused on port 443") {
		t.Fatalf("addr %q line %q", addr, got)
	}
}

func TestNameLineRealRefusal(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	got := NameLine(Refused, "http://localhost:"+itoa(port), outbound.PolicyDialer(outbound.NotMetadata).DialContext)
	if !strings.HasPrefix(got, "localhost → ") || !strings.Contains(got, "refused on port "+itoa(port)) {
		t.Fatalf("got %q", got)
	}
	// The public-only rule refuses a local address, so nothing is named.
	if got := NameLine(Refused, "http://localhost:"+itoa(port), outbound.PolicyDialer(outbound.PublicOnly).DialContext); got != "" {
		t.Fatalf("public-only got %q", got)
	}
}

func TestNameLinesReadBackAsTheirCause(t *testing.T) {
	for line, c := range map[string]Cause{
		"qbittorrent → not found. HolyArrs can't find this name. Use the IP instead.":                                                                 DNS,
		"qbittorrent → 172.18.0.5 → refused on port 8080. The name works, but nothing answers on that port. Check the port.":                          Refused,
		"sab → 172.18.0.5 → no answer on port 8080. The name works, but nothing answers in time. Check the port, and that no firewall is in the way.": Timeout,
	} {
		if got := OfText(line); got != c {
			t.Fatalf("%q read as %q, want %q", line, got, c)
		}
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

func TestNameLineWaitsLessAfterATimeout(t *testing.T) {
	var left time.Duration
	dial := func(ctx context.Context, _, _ string) (net.Conn, error) {
		d, _ := ctx.Deadline()
		left = time.Until(d)
		return nil, &net.OpError{Op: "dial", Net: "tcp", Addr: &net.TCPAddr{IP: net.ParseIP("10.0.0.2"), Port: 80}, Err: context.DeadlineExceeded}
	}
	NameLine(Timeout, "http://sab:80", dial)
	if left > timeoutLook {
		t.Fatalf("waits %v after a timeout", left)
	}
}
