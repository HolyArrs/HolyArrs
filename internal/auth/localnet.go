package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
)

// keyLocalNets holds the networks where sign-in may be skipped, as a JSON
// list of CIDRs.
const keyLocalNets = "local_networks"

// maxLocalNets caps the networks the owner may list.
const maxLocalNets = 32

// localNets is the owner's list as typed (cleaned up) and as parsed.
type localNets struct {
	list []string
	nets []*net.IPNet
}

// parseNet reads one CIDR or single address; an address is its own network.
func parseNet(e string) (*net.IPNet, error) {
	if !strings.Contains(e, "/") {
		ip := net.ParseIP(strings.Trim(e, "[]"))
		if ip == nil {
			return nil, errors.New("not an address")
		}
		if v4 := ip.To4(); v4 != nil {
			return &net.IPNet{IP: v4, Mask: net.CIDRMask(32, 32)}, nil
		}
		return &net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)}, nil
	}
	_, n, err := net.ParseCIDR(e)
	if err != nil {
		return nil, err
	}
	if ones, _ := n.Mask.Size(); ones == 0 {
		return nil, errors.New("that is every address")
	}
	return n, nil
}

// parseLocalNets cleans up the owner's list: spaces trimmed, empty entries
// and repeats dropped. It names the first entry it cannot read.
func parseLocalNets(in []string) (*localNets, error) {
	out := &localNets{list: []string{}}
	seen := map[string]bool{}
	for _, e := range in {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		n, err := parseNet(e)
		if err != nil {
			return nil, fmt.Errorf("%q is not a network: use one like 192.168.1.0/24 or a single address like 192.168.1.5", truncate(e, 60))
		}
		if s := n.String(); !seen[s] {
			seen[s] = true
			out.list = append(out.list, s)
			out.nets = append(out.nets, n)
		}
	}
	if len(out.list) > maxLocalNets {
		return nil, fmt.Errorf("list at most %d networks", maxLocalNets)
	}
	return out, nil
}

// loadLocalNets reads the saved list. Entries it cannot read are skipped,
// so a damaged value trusts less, never more.
func (a *Auth) loadLocalNets(ctx context.Context) *localNets {
	v := a.signInSetting(ctx, keyLocalNets)
	var in []string
	if err := json.Unmarshal([]byte(v), &in); err != nil {
		in = strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == '\n' || r == ' ' })
	}
	out := &localNets{list: []string{}}
	for _, e := range in {
		if n, err := parseNet(strings.TrimSpace(e)); err == nil && len(out.list) < maxLocalNets {
			out.list = append(out.list, n.String())
			out.nets = append(out.nets, n)
		} else if strings.TrimSpace(e) != "" {
			log.Printf("auth: ignoring local network %q", e)
		}
	}
	return out
}

// isLocal reports whether the peer address is inside a network the owner
// listed for skipping sign-in.
func (a *Auth) isLocal(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	l := a.localNets.Load()
	if ip == nil || l == nil {
		return false
	}
	for _, n := range l.nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// containerNets are networks Docker, Podman and Docker Desktop hand out by
// default. Behind them every browser can show up as one gateway address.
var containerNets = func() []*net.IPNet {
	var out []*net.IPNet
	for _, s := range []string{"172.17.0.0/16", "10.88.0.0/16", "10.0.2.0/24", "192.168.65.0/24"} {
		_, n, _ := net.ParseCIDR(s)
		out = append(out, n)
	}
	return out
}()

// gatewayLike says whether an address looks like a container gateway:
// 172.x.0.1 or 10.88.0.1 bridges, 10.0.2.2 (rootless) or Docker Desktop's
// 192.168.65.x.
func gatewayLike(ip net.IP) bool {
	v4 := ip.To4()
	if v4 == nil {
		return false
	}
	switch {
	case v4[0] == 192 && v4[1] == 168 && v4[2] == 65:
		return true
	case v4[0] == 10 && v4[1] == 0 && v4[2] == 2 && v4[3] == 2:
		return true
	case (v4[0] == 172 && v4[1]&0xf0 == 16 || v4[0] == 10 && v4[1] == 88) && v4[2] == 0 && v4[3] == 1:
		return true
	}
	return false
}

// gatewayNets lists the entries that look like a container network, or
// that hold self (the address this browser shows up as) when self looks
// like a gateway. It only feeds a warning; nothing is refused.
func gatewayNets(list []string, self string) []string {
	out := []string{}
	me := net.ParseIP(strings.Trim(self, "[]"))
	for _, e := range list {
		n, err := parseNet(e)
		if err != nil {
			continue
		}
		ones, bits := n.Mask.Size()
		hit := me != nil && gatewayLike(me) && n.Contains(me)
		for _, c := range containerNets {
			if cOnes, cBits := c.Mask.Size(); cBits == bits && ones >= cOnes && c.Contains(n.IP) {
				hit = true
			}
		}
		if hit {
			out = append(out, e)
		}
	}
	return out
}

// seenAddress is the address the local-network check reads for this
// request: the forwarded client behind a trusted proxy, otherwise the peer.
func (a *Auth) seenAddress(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer := net.ParseIP(strings.Trim(host, "[]"))
	if peer == nil || !a.trustedProxy(peer) || r.Header.Get("Forwarded") != "" {
		return host
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[len(parts)-1])
	}
	if real := strings.TrimSpace(r.Header.Get("X-Real-IP")); real != "" {
		return real
	}
	return host
}
