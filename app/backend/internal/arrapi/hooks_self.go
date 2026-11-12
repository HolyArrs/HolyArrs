package arrapi

import (
	"net"
	"net/http"
	"strings"
	"sync/atomic"
)

// listenPort is the port HolyArrs listens on, "" before it is known.
var listenPort atomic.Value

// SetListenAddr tells hooks the address HolyArrs listens on, so a hook
// can never call HolyArrs itself.
func SetListenAddr(addr string) {
	_, port, _ := net.SplitHostPort(addr)
	listenPort.Store(port)
}

// selfAddr reports whether ip:port is HolyArrs itself: its listen port on
// loopback, an unspecified address or one of this machine's own addresses.
func selfAddr(ip net.IP, port string) bool {
	own, _ := listenPort.Load().(string)
	if own == "" || port != own {
		return false
	}
	if ip.IsLoopback() || ip.IsUnspecified() {
		return true
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return true // cannot tell: refuse rather than risk calling itself
	}
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && n.IP.Equal(ip) {
			return true
		}
	}
	return false
}

// reachedOn reports whether the hook address names the host and port the
// caller reached HolyArrs on.
func reachedOn(r *http.Request, hookURL string) bool {
	u, err := http.NewRequest(http.MethodGet, hookURL, nil)
	if err != nil || r.Host == "" {
		return false
	}
	return withPort(u.URL.Host, u.URL.Scheme) == withPort(r.Host, schemeOf(r))
}

func schemeOf(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

// withPort lowercases host and adds the scheme's default port when none is given.
func withPort(host, scheme string) string {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if _, _, err := net.SplitHostPort(host); err == nil {
		return host
	}
	if scheme == "https" {
		return net.JoinHostPort(strings.Trim(host, "[]"), "443")
	}
	return net.JoinHostPort(strings.Trim(host, "[]"), "80")
}
