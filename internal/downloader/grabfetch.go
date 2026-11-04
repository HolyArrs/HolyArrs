package downloader

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs/internal/source"
)

// errLinkLocal is a Release link that leads to this machine or the local
// network, found when its address is dialled.
var errLinkLocal = errors.New("the Release link leads to a local or blocked address")

// sabMaxNZB caps an NZB fetched for SABnzbd: a few hundred GB of release.
var sabMaxNZB int64 = 128 << 20

// grabAllow is the address rule for Release files HolyArrs fetches in place
// of qBittorrent or SABnzbd. grabDial, when set (tests only), changes the
// address dialled for a name, as a DNS answer would.
var (
	grabAllow = source.PublicOnly
	grabDial  func(addr string) string
)

// fetchGrabLink fetches a Release link for a Downloader that would
// otherwise fetch it itself, from inside the home network (#152). Every
// address is checked as it is dialled, after DNS and on each redirect, so a
// name or a redirect that leads home is refused. A redirect to a magnet
// link returns that link instead of a file, as Sonarr does. rt, when set,
// is the Source's own checked transport (through its proxy).
func fetchGrabLink(ctx context.Context, raw string, limit int64, rt http.RoundTripper) (file []byte, magnet string, err error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, "", errors.New("the Release file needs an http or https address without a sign-in")
	}
	d := source.PolicyDialer(grabAllow)
	dial := func(ctx context.Context, network, addr string) (net.Conn, error) {
		if h, _, err := net.SplitHostPort(addr); err == nil && metadataName(h) {
			return nil, &net.OpError{Op: "dial", Net: network, Err: source.ErrAddressRefused}
		}
		if grabDial != nil {
			addr = grabDial(addr)
		}
		return d.DialContext(ctx, network, addr)
	}
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
	resp, err := h.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
		if errors.Is(err, source.ErrAddressRefused) || errors.Is(err, errLinkLocal) {
			return nil, "", errLinkLocal
		}
		return nil, "", errors.New("the Release file could not be read from its Source")
	}
	defer resp.Body.Close()
	if loc := resp.Header.Get("Location"); resp.StatusCode >= 300 && resp.StatusCode < 400 && len(loc) >= 7 && strings.EqualFold(loc[:7], "magnet:") {
		return nil, loc, nil
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
