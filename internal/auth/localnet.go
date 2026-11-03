package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
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
