package sources

import (
	"context"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

// BridgeIsLocal is the check Sources use for a local address; tests replace it.
var BridgeIsLocal = BridgeLocalAddress

// BridgeLocalAddress says whether an address is on this machine or a local
// network (loopback, private and the other ranges Sources refuse). Only a
// Jackett there serves its sites' feeds from there too, so only then do its
// Sources get the local-network allowance. A name counts as local only when
// every address it resolves to is. ok is false when the name could not be
// looked up.
func BridgeLocalAddress(ctx context.Context, raw string) (local, ok bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return false, false
	}
	host := u.Hostname()
	if ip := net.ParseIP(host); ip != nil {
		return !outbound.PublicOnly(ip), true
	}
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return true, true
	}
	// A slow name server must not stall a check.
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(ips) == 0 {
		return false, false
	}
	for _, ip := range ips {
		if outbound.PublicOnly(ip.IP) {
			return false, true
		}
	}
	return true, true
}
