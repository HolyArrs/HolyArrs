package accounts

import (
	"bufio"
	"encoding/hex"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

// When HolyArrs runs on a Docker bridge, a browser that opens the host's LAN
// IP shows up as the container's default gateway. While the owner has the
// local skip on, that one address counts as local. It is read from
// the container's own routing table, never guessed from the address shape.

// readGateways returns the default gateways of this machine; tests swap it.
var readGateways = func() []net.IP {
	var out []net.IP
	if f, err := os.Open("/proc/net/route"); err == nil {
		out = append(out, parseRoute(f)...)
		f.Close()
	}
	if f, err := os.Open("/proc/net/ipv6_route"); err == nil {
		out = append(out, parseRoute6(f)...)
		f.Close()
	}
	return out
}

// gatewayEvery is how often the gateways are read again.
const gatewayEvery = 5 * time.Minute

var gateways struct {
	sync.Mutex
	ips []net.IP
	at  time.Time
}

// resetGateways forgets the cached gateways.
func resetGateways() {
	gateways.Lock()
	gateways.ips, gateways.at = nil, time.Time{}
	gateways.Unlock()
}

// ownGateway says whether ip is one of this container's default gateways.
func ownGateway(ip net.IP) bool {
	if ip == nil {
		return false
	}
	gateways.Lock()
	if gateways.at.IsZero() || time.Since(gateways.at) > gatewayEvery {
		gateways.ips, gateways.at = readGateways(), time.Now()
	}
	ips := gateways.ips
	gateways.Unlock()
	for _, g := range ips {
		if g.Equal(ip) {
			return true
		}
	}
	return false
}

// parseRoute reads /proc/net/route: the gateway of each default route
// (destination and mask 0), stored as little-endian hex.
func parseRoute(r io.Reader) []net.IP {
	var out []net.IP
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 8 || f[1] != "00000000" || f[7] != "00000000" {
			continue
		}
		b, err := hex.DecodeString(f[2])
		if err != nil || len(b) != 4 {
			continue
		}
		ip := net.IPv4(b[3], b[2], b[1], b[0])
		if !ip.IsUnspecified() {
			out = append(out, ip)
		}
	}
	return out
}

// parseRoute6 reads /proc/net/ipv6_route: the next hop of each default
// route (destination ::/0).
func parseRoute6(r io.Reader) []net.IP {
	var out []net.IP
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 5 || f[0] != strings.Repeat("0", 32) || f[1] != "00" {
			continue
		}
		b, err := hex.DecodeString(f[4])
		if err != nil || len(b) != 16 {
			continue
		}
		ip := net.IP(b)
		if !ip.IsUnspecified() {
			out = append(out, ip)
		}
	}
	return out
}
