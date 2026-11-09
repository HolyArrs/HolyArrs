package metadata

import (
	"context"
	"net"
	"net/http"
	"time"
)

// NewServerTransport is the one connection pool an install keeps to the
// metadata server: HTTP/2 over TLS (a custom dialer turns Go's HTTP/2 off
// unless asked for), connections kept open between requests, so text
// answers and pictures share one warm connection instead of a TLS setup
// each. dial is the address policy (public addresses only, or the local
// network for a lab server).
func NewServerTransport(dial func(ctx context.Context, network, addr string) (net.Conn, error)) *http.Transport {
	return &http.Transport{
		Proxy:                  nil, // the policy must see the real address
		DialContext:            dial,
		ForceAttemptHTTP2:      true,
		MaxIdleConns:           32,
		MaxIdleConnsPerHost:    16,
		MaxConnsPerHost:        16,
		IdleConnTimeout:        90 * time.Second,
		TLSHandshakeTimeout:    10 * time.Second,
		ResponseHeaderTimeout:  16 * time.Second,
		ExpectContinueTimeout:  time.Second,
		MaxResponseHeaderBytes: 32 << 10,
	}
}

// HostRoute sends requests for one host through its own transport (the
// metadata server's pool) and every other request through rest.
type HostRoute struct {
	Host string
	Via  http.RoundTripper
	Rest http.RoundTripper
}

func (h HostRoute) RoundTrip(req *http.Request) (*http.Response, error) {
	if h.Via != nil && h.Host != "" && req.URL.Host == h.Host {
		return h.Via.RoundTrip(req)
	}
	return h.Rest.RoundTrip(req)
}
