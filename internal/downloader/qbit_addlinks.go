package downloader

import (
	"context"
	"errors"
	"net"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"github.com/CoderLuii/holyarrs/internal/source"
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
			if !allowLocal {
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
	if err != nil || u.Hostname() == "" || u.User != nil {
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
	if ip := net.ParseIP(host); ip != nil {
		if !source.NotMetadata(ip) {
			return false
		}
		return allowLocal || source.PublicOnly(ip)
	}
	if !allowLocal && (host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local")) {
		return false
	}
	return true
}
