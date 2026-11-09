package downloads

import (
	"context"
	"errors"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

// addFetchMax caps a fetched .torrent at 10 MiB. Real ones are tiny, so a bigger answer is not a torrent. It is a variable so tests can lower it.
var addFetchMax int64 = 10 << 20

// errMagnet is the one error for any bad magnet, so the reason does not leak what was wrong.
var errMagnet = errors.New("the magnet link could not be read")

// btihHex, btihB32 and btmh match the three info hash forms a magnet may carry: v1 hex, v1 base32 and v2 multihash.
var btihHex = regexp.MustCompile(`^[0-9a-f]{40}$`)

var btihB32 = regexp.MustCompile(`^[a-z2-7]{32}$`)

var btmh = regexp.MustCompile(`^1220[0-9a-f]{64}$`)

// fetchTorrentLink downloads a .torrent from a web link under the Source address rules and checks it parses.
func fetchTorrentLink(ctx context.Context, link string, allowLocal bool) ([]byte, error) {
	b, err := fetchRelease(ctx, link, allowLocal, addFetchMax)
	if err != nil {
		return nil, err
	}
	if _, err := torrentFileHash(b); err != nil {
		return nil, err
	}
	return b, nil
}

// safeMagnet rebuilds a magnet link from parts known to be safe and counts what it dropped.
// Only the hash, display name and allowed trackers and peers survive. Unknown keys go, so the client never gets a stray address to contact. Local addresses pass only when the Source may use the local network.
// Control characters in any part reject the link.
func safeMagnet(link string, allowLocal bool) (string, int, error) {
	if len(link) < 8 || !strings.EqualFold(link[:8], "magnet:?") {
		return "", 0, errMagnet
	}
	type pair struct{ k, v string }
	var keep []pair
	dropped, hashes := 0, 0
	for _, raw := range strings.Split(link[8:], "&") {
		if raw == "" {
			continue
		}
		rk, rv, _ := strings.Cut(raw, "=")
		k, err1 := url.QueryUnescape(rk)
		v, err2 := url.QueryUnescape(rv)
		if err1 != nil || err2 != nil || strings.IndexFunc(k+v, unicode.IsControl) >= 0 {
			return "", 0, errMagnet
		}
		k = strings.ToLower(k)
		switch k {
		case "xt":
			h := strings.ToLower(v)
			if b, ok := strings.CutPrefix(h, "urn:btih:"); ok && (btihHex.MatchString(b) || btihB32.MatchString(b)) {
				hashes++
			} else if b, ok := strings.CutPrefix(h, "urn:btmh:"); ok && btmh.MatchString(b) {
				hashes++
			} else {
				return "", 0, errMagnet
			}
		// The display name is only a label and is kept as is.
		case "dn":
		case "tr":
			if !trackerAllowed(v, allowLocal) {
				dropped++
				continue
			}
		case "ws", "xs", "as", "x.pe":
			if !allowLocal || !peerAllowed(k, v) {
				dropped++
				continue
			}
		default:
			dropped++
			continue
		}
		keep = append(keep, pair{k, v})
	}
	if hashes == 0 {
		return "", 0, errMagnet
	}
	parts := make([]string, len(keep))
	for i, p := range keep {
		if p.k == "xt" {
			parts[i] = "xt=" + strings.ToLower(p.v)
			continue
		}
		parts[i] = p.k + "=" + url.QueryEscape(p.v)
	}
	return "magnet:?" + strings.Join(parts, "&"), dropped, nil
}

// trackerAllowed accepts http, https and udp trackers on public hosts. Cloud metadata addresses are never allowed.
// User info and odd forms of an address are refused, since the client and this check could read them differently.
func trackerAllowed(v string, allowLocal bool) bool {
	u, err := url.Parse(v)
	if err != nil || u.Hostname() == "" || u.User != nil || oddAuthority(v) {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "udp":
	default:
		return false
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if metadataName(host) {
		return false
	}
	if ip, numeric := hostIP(host); ip != nil {
		if !outbound.NotMetadata(ip) {
			return false
		}
		return allowLocal || outbound.PublicOnly(ip)
	} else if numeric {
		return false
	}
	if !allowLocal && oddHost(host) {
		return false
	}
	if !allowLocal && (host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local")) {
		return false
	}
	return true
}

// oddHost spots hosts that are numbers in disguise (hex, octal, short forms) or have no letter in the last label. Some resolvers turn those into private addresses.
func oddHost(host string) bool {
	if strings.Contains(host, "%") {
		return true
	}
	labels := strings.Split(host, ".")
	numeric := true
	for _, l := range labels {
		if !numberLabel(l) {
			numeric = false
		}
	}
	if numeric {
		return true
	}
	tld := labels[len(labels)-1]
	if tld == "" || numberLabel(tld) {
		return true
	}
	for _, c := range tld {
		if c >= 'a' && c <= 'z' {
			return false
		}
	}
	return true
}

// numberLabel reports whether a host label is a decimal or 0x hex number, or empty.
func numberLabel(l string) bool {
	if l == "" {
		return true
	}
	if h, ok := strings.CutPrefix(l, "0x"); ok {
		for _, c := range h {
			if !strings.ContainsRune("0123456789abcdef", c) {
				return false
			}
		}
		return true
	}
	for _, c := range l {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// peerAllowed checks web seed and peer hints. They are only kept when local use is on, and never when they point at metadata addresses or a malformed number.
func peerAllowed(k, v string) bool {
	var host string
	if k == "x.pe" {
		h, _, err := net.SplitHostPort(v)
		if err != nil {
			return false
		}
		host = h
	} else {
		u, err := url.Parse(v)
		if err != nil || u.Hostname() == "" || oddAuthority(v) {
			return false
		}
		host = u.Hostname()
	}
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if metadataName(host) {
		return false
	}
	ip, numeric := hostIP(host)
	if ip != nil {
		return outbound.NotMetadata(ip)
	}
	return !numeric
}

// hostIP reads an address the way old C resolvers do, including short and hex forms such as 127.1 or 0x7f.0.0.1. numeric is true when the host is made of numbers even if it is not a valid address, so the caller can refuse it.
func hostIP(host string) (ip net.IP, numeric bool) {
	h := strings.Trim(host, "[]")
	if i := strings.IndexByte(h, '%'); i >= 0 && strings.Contains(h, ":") {
		h = h[:i]
	}
	if strings.Contains(h, ":") {
		if ip := net.ParseIP(h); ip != nil {
			return ip, true
		}
		return nil, true
	}
	parts := strings.Split(h, ".")
	all := true
	for _, p := range parts {
		all = all && numberLabel(p)
	}
	if !all {
		return nil, false
	}
	if len(parts) > 4 {
		return nil, true
	}
	vals := make([]uint64, len(parts))
	for i, p := range parts {
		v, err := strconv.ParseUint(p, 0, 32)
		if err != nil || p == "" {
			return nil, true
		}
		vals[i] = v
	}
	last := len(vals) - 1
	for i := 0; i < last; i++ {
		if vals[i] > 255 {
			return nil, true
		}
	}
	// The last part fills all the remaining bytes, so it has a bigger limit.
	if vals[last] >= 1<<(8*(4-last)) {
		return nil, true
	}
	var n uint64
	for i := 0; i < last; i++ {
		n |= vals[i] << (8 * (3 - i))
	}
	n |= vals[last]
	return net.IPv4(byte(n>>24), byte(n>>16), byte(n>>8), byte(n)), true
}
