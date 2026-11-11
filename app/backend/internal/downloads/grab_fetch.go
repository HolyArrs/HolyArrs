package downloads

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

var errLinkLocal = releaseRefused{`the Release link leads to a local or blocked address; if this Source is on your home network, turn on "May reach the local network" for it`}

var sabMaxNZB int64 = 128 << 20

var (
	grabAllow = outbound.PublicOnly
	grabDial  func(addr string) string
)

func fetchGrabLink(ctx context.Context, raw string, limit int64, rt http.RoundTripper, localHost string) (file []byte, magnet string, err error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, "", errors.New("the Release file needs an http or https address without a sign-in")
	}
	d, home := outbound.PolicyDialer(grabAllow), outbound.PolicyDialer(outbound.NotMetadata)
	dial := func(ctx context.Context, network, addr string) (net.Conn, error) {
		h, port, err := net.SplitHostPort(addr)
		if err == nil && metadataName(h) {
			return nil, &net.OpError{Op: "dial", Net: network, Err: outbound.ErrAddressRefused}
		}
		if grabDial != nil {
			addr = grabDial(addr)
		}
		if localHost != "" && err == nil && strings.EqualFold(net.JoinHostPort(strings.TrimSuffix(h, "."), port), localHost) {
			return home.DialContext(ctx, network, addr)
		}
		return d.DialContext(ctx, network, addr)
	}
	proxied := rt != nil
	if rt == nil {
		rt = &http.Transport{Proxy: nil, DialContext: dial, TLSHandshakeTimeout: 10 * time.Second}
	}
	h := &http.Client{
		Timeout:   clientTimeout,
		Transport: rt,
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			if strings.EqualFold(r.URL.Scheme, "magnet") {
				return http.ErrUseLastResponse
			}
			if (r.URL.Scheme != "http" && r.URL.Scheme != "https") || r.URL.User != nil {
				return errLinkLocal
			}
			return nil
		},
	}
	defer h.CloseIdleConnections()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, "", errors.New("the Release file address is invalid")
	}
	// Sites behind Cloudflare refuse Go's default User-Agent; a proxied
	// Source's transport would add this too, but a plain one would not.
	req.Header.Set("User-Agent", sources.UserAgent)
	resp, err := h.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
		if errors.Is(err, outbound.ErrAddressRefused) || errors.Is(err, errLinkLocal) {
			if proxied {
				return nil, "", errors.New(errLinkLocal.msg)
			}
			return nil, "", errLinkLocal
		}
		return nil, "", errors.New("the Release file could not be read from its Source")
	}
	defer resp.Body.Close()
	if loc := resp.Header.Get("Location"); resp.StatusCode >= 300 && resp.StatusCode < 400 && len(loc) >= 7 && strings.EqualFold(loc[:7], "magnet:") {
		return nil, loc, nil
	}
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		return nil, "", releaseRefused{"the Source has no such Release file"}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, "", errors.New("the Source refused the Release file")
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
		return nil, "", errors.New("the Release file could not be read completely")
	}
	if len(b) == 0 || int64(len(b)) > limit {
		return nil, "", errors.New("the Release file is empty or too large")
	}
	return b, "", nil
}

// sourceLocalHost is the host and port a Source on the home network may be
// fetched from; other local addresses stay refused.
func sourceLocalHost(sourceURL string, allowLocal bool) string {
	su, err := url.Parse(sourceURL)
	if !allowLocal || err != nil || su.Hostname() == "" {
		return ""
	}
	port := su.Port()
	if port == "" {
		port = map[string]string{"http": "80", "https": "443"}[strings.ToLower(su.Scheme)]
	}
	return net.JoinHostPort(strings.TrimSuffix(strings.ToLower(su.Hostname()), "."), port)
}

// prefetchTorrent reads a torrent link without following a magnet redirect. It
// returns the magnet, or the link with the file, or the link alone if the read fails.
func prefetchTorrent(ctx context.Context, link string, allowLocal bool, localHost string) (string, []byte) {
	file, magnet, err := fetchGrabLink(ctx, link, torrentFetchMax, nil, localHost)
	if err != nil {
		return link, nil
	}
	if magnet != "" {
		if clean, _, err := safeMagnet(magnet, allowLocal); err == nil {
			return clean, nil
		}
		return link, nil
	}
	return link, file
}

// torrentFetchMax caps a .torrent read for Transmission, Deluge and rTorrent.
const torrentFetchMax int64 = 8 << 20
