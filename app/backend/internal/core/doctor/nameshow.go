package doctor

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

// The name lines, with what the address's name turned into.
const (
	nameNotFound = "{name} → not found. HolyArrs can't find this name. Use the IP instead."
	nameRefused  = "{name} → {ip} → refused on port {port}. The name works, but nothing answers on that port. Check the port."
	nameNoAnswer = "{name} → {ip} → no answer on port {port}. The name works, but nothing answers in time. Check the port, and that no firewall is in the way."
)

// NameLine is the line for a failed call to rawURL that shows what its host
// name turned into, "" when the host is already an IP, the cause is not about
// reaching the address, or the name step shows nothing new. It looks only
// through dial, the caller's own policy-checked way out, so the same network
// rule holds; with no dial (a proxy carries the calls) it does not look.
func NameLine(c Cause, rawURL string, dial Dial) string {
	if dial == nil || c != DNS && c != Refused && c != Timeout {
		return ""
	}
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	host := u.Hostname()
	if host == "" || net.ParseIP(host) != nil {
		return ""
	}
	port := u.Port()
	if port == "" {
		port = map[string]string{"http": "80", "https": "443"}[strings.ToLower(u.Scheme)]
	}
	if c == DNS && !localName(host) {
		// A public name that is not found is a typo, not a reason to use an IP.
		return ""
	}
	wait := probeWait
	if c == Timeout {
		// The call already waited its full time; a short second wait is enough
		// to see where the name leads.
		wait = timeoutLook
	}
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	conn, err := dial(ctx, "tcp", net.JoinHostPort(host, port))
	if err == nil {
		conn.Close()
		return ""
	}
	return nameLineOf(host, err)
}

// timeoutLook bounds the name look after a call that timed out.
const timeoutLook = 1500 * time.Millisecond

// localSuffixes end names that only a home network knows.
var localSuffixes = []string{".local", ".lan", ".home", ".home.arpa", ".internal", ".localdomain", ".ts.net"}

// localName says whether host is a name only a home network, Docker or a
// tailnet can know: a bare container name or one of localSuffixes.
func localName(host string) bool {
	h := strings.ToLower(strings.TrimSuffix(host, "."))
	if !strings.Contains(h, ".") {
		return true
	}
	for _, s := range localSuffixes {
		if strings.HasSuffix(h, s) {
			return true
		}
	}
	return false
}

func nameLineOf(host string, err error) string {
	if errors.Is(err, outbound.ErrAddressRefused) {
		return ""
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		if dnsErr.IsNotFound {
			return strings.Replace(nameNotFound, "{name}", host, 1)
		}
		return ""
	}
	var op *net.OpError
	if !errors.As(err, &op) || op.Addr == nil {
		return ""
	}
	ip, port, splitErr := net.SplitHostPort(op.Addr.String())
	if splitErr != nil {
		return ""
	}
	if _, convErr := strconv.Atoi(port); convErr != nil {
		return ""
	}
	line := ""
	switch Of(err) {
	case Refused:
		line = nameRefused
	case Timeout:
		line = nameNoAnswer
	default:
		return ""
	}
	return strings.NewReplacer("{name}", host, "{ip}", ip, "{port}", port).Replace(line)
}
