package downloader

import (
	"context"
	"net"
	"net/url"
	"strings"

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

// magnetURLParams are the magnet parameters that carry an address the
// client would contact: trackers, web seeds, exact sources, acceptable
// sources and peer addresses.
var magnetURLParams = []string{"tr", "ws", "xs", "as", "x.pe"}

// safeMagnet drops every address in a magnet link that points at this
// machine, the local network or a metadata service, unless the Downloader
// may use the local network (metadata addresses are always dropped). It
// says how many it dropped.
func safeMagnet(ctx context.Context, link string, allowLocal bool) (string, int) {
	u, err := url.Parse(link)
	if err != nil {
		return link, 0
	}
	q := u.Query()
	dropped := 0
	for _, k := range magnetURLParams {
		vals, ok := q[k]
		if !ok {
			continue
		}
		keep := vals[:0]
		for _, v := range vals {
			if addressAllowed(ctx, k, v, allowLocal) {
				keep = append(keep, v)
			} else {
				dropped++
			}
		}
		if len(keep) == 0 {
			q.Del(k)
		} else {
			q[k] = keep
		}
	}
	if dropped == 0 {
		return link, 0
	}
	u.RawQuery = q.Encode()
	return u.String(), dropped
}

// addressAllowed checks one magnet address by its host: every address it
// names or resolves to must pass the Downloader's policy.
func addressAllowed(ctx context.Context, key, v string, allowLocal bool) bool {
	host := ""
	if key == "x.pe" {
		h, _, err := net.SplitHostPort(v)
		if err != nil {
			return false
		}
		host = h
	} else {
		u, err := url.Parse(v)
		if err != nil || u.Hostname() == "" {
			return false
		}
		host = u.Hostname()
	}
	if metadataName(host) {
		return false
	}
	allow := source.NotMetadata
	if !allowLocal {
		allow = source.PublicOnly
	}
	if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
		return allow(ip)
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(addrs) == 0 {
		return false
	}
	for _, a := range addrs {
		if !allow(a.IP) {
			return false
		}
	}
	return true
}
