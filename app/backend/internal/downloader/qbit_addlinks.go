package downloader

import (
	"context"
	"errors"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/HolyArrs/holyarrs/app/backend/internal/source"
)

// addFetchMax caps a .torrent file HolyArrs fetches for a hand-made add.
var addFetchMax int64 = 10 << 20

// fetchTorrentLink fetches an http(s) .torrent link itself, through the
// Downloader's own client policy (fetchRelease: every dialled address is
// checked, redirects stay on the same host), so the client is never told
// to fetch an address a person typed.
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

// errMagnet is a magnet link that does not parse strictly.
var errMagnet = errors.New("the magnet link could not be read")

var btihHex = regexp.MustCompile(`^[0-9a-f]{40}$`)
var btihB32 = regexp.MustCompile(`^[a-z2-7]{32}$`)
var btmh = regexp.MustCompile(`^1220[0-9a-f]{64}$`)

// safeMagnet rebuilds a magnet link from an allowlist: xt (a BitTorrent
// v1 or v2 hash), dn and tr; with local network access also ws, xs, as and
// x.pe. Every pair is decoded strictly and the link is written anew, so
// the original string never reaches the client. A pair that does not
// decode, or holds a control character, refuses the whole link. It says
// how many pairs it left out.
//
// Trackers are kept by host name, as for trackers inside an uploaded
// .torrent file: qBittorrent announces to them itself, so a name cannot be
// checked here in a way that survives rebinding. Without local network
// access a tracker that is a literal local, link-local or metadata address,
// localhost or a .local name, or that is not http, https or udp, is left
// out. Nothing here looks names up.
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
			parts[i] = "xt=" + strings.ToLower(p.v) // checked above: letters, digits and colons only
			continue
		}
		parts[i] = p.k + "=" + url.QueryEscape(p.v)
	}
	return "magnet:?" + strings.Join(parts, "&"), dropped, nil
}

// trackerAllowed checks one magnet tracker without looking its name up.
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
		if !source.NotMetadata(ip) {
			return false
		}
		return allowLocal || source.PublicOnly(ip)
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

// oddHost is a host that is not a plain public name: a percent sign, labels
// that are all digits or hex numbers (127.1, 2130706433, 0x7f.1, 017700000001,
// which some resolvers read as addresses), or no alphabetic last label.
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

// numberLabel is a label that reads as a decimal, octal or 0x hex number.
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

// peerAllowed checks a web seed, source or peer pair when local network
// access is on: cloud metadata targets stay refused anyway.
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
		return source.NotMetadata(ip)
	}
	return !numeric
}

// hostIP reads a host as an address the way inet_aton and resolvers do:
// IPv6 (zone dropped) or one to four dotted parts in decimal, octal or 0x
// hex. numeric says the host looks like a number form even when it fails
// to parse (such a host is refused).
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
