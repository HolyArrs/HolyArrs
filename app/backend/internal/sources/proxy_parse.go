package sources

import (
	"net/url"
	"strconv"
	"strings"
)

// parseProxy checks a proxy address and returns it without the password, which comes back apart.
func parseProxy(raw string) (clean string, pass string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5") || u.Port() == "" || u.Hostname() == "" ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" {
		return "", "", ErrProxy
	}
	if n, err := strconv.Atoi(u.Port()); err != nil || n < 1 || n > 65535 {
		return "", "", ErrProxy
	}
	out := url.URL{Scheme: u.Scheme, Host: u.Host}
	if u.User != nil {
		pass, _ = u.User.Password()
		out.User = url.User(u.User.Username())
	}
	return out.String(), pass, nil
}
