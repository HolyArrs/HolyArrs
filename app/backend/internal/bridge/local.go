package bridge

import (
	"context"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/source"
)

// IsLocal is LocalAddress; tests swap it.
var IsLocal = LocalAddress

// LocalAddress says whether an address is on this machine or a local
// network (loopback, private and the other ranges Sources refuse). A
// Jackett there serves its sites' feeds from there too, so only then do its
// Sources get the local-network allowance. A name is looked up, and counts
// as local only when every address it has is. ok is false when the name
// could not be looked up.
func LocalAddress(ctx context.Context, raw string) (local, ok bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return false, false
	}
	host := u.Hostname()
	if ip := net.ParseIP(host); ip != nil {
		return !source.PublicOnly(ip), true
	}
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return true, true
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(ips) == 0 {
		return false, false
	}
	for _, ip := range ips {
		if source.PublicOnly(ip.IP) {
			return false, true
		}
	}
	return true, true
}
