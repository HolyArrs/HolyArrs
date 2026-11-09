package accounts

import (
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

const oidcBodyLimit = 1 << 20

var (
	errOIDCBodyLarge  = errors.New("OIDC response was too large")
	errOIDCRedirect   = errors.New("OIDC redirect refused")
	errOIDCSuppressed = errors.New("OIDC provider is temporarily unavailable")
)

type oidcLimitTransport struct {
	base             http.RoundTripper
	issuer, clientID string
}

func (t oidcLimitTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	targetURL := *req.URL
	targetURL.User, targetURL.RawQuery, targetURL.ForceQuery, targetURL.Fragment = nil, "", false, ""
	op, err := outbound.Begin(req.Context(), outbound.Target{Kind: "oidc", Identity: []string{t.issuer, t.clientID, targetURL.String()}})
	if err != nil {
		return nil, errOIDCSuppressed
	}
	ctx, cancel := op.AttemptContext(req.Context(), 10*time.Second)
	defer cancel()
	resp, callErr := t.base.RoundTrip(req.Clone(ctx))
	status := 0
	if resp != nil {
		status = resp.StatusCode
	}
	if callErr != nil {
		op.Finish(outbound.HTTPOutcome(status, callErr, req.Context()))
		return nil, callErr
	}
	defer resp.Body.Close()
	if resp.ContentLength > oidcBodyLimit {
		op.Finish(outbound.HTTPOutcome(status, errOIDCBodyLarge, req.Context()))
		return nil, errOIDCBodyLarge
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, oidcBodyLimit+1))
	if readErr == nil && len(body) > oidcBodyLimit {
		readErr = errOIDCBodyLarge
	}
	op.Finish(outbound.HTTPOutcome(status, readErr, req.Context()))
	if readErr != nil {
		return nil, readErr
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	return resp, nil
}

func newOIDCHTTPClient(opt OIDCOptions, issuer *url.URL) *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil
	t.DialContext = outbound.PolicyDialer(func(ip net.IP) bool { return oidcAddressAllowed(opt, ip) }).DialContext
	return &http.Client{
		Timeout:   10 * time.Second,
		Transport: oidcLimitTransport{base: t, issuer: issuer.String(), clientID: opt.ClientID},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errOIDCRedirect
		},
	}
}

func oidcAddressAllowed(opt OIDCOptions, ip net.IP) bool {
	if ip == nil || ip.IsUnspecified() || ip.IsMulticast() || ip.IsInterfaceLocalMulticast() || ip.Equal(net.IPv4bcast) {
		return false
	}
	if opt.AllowLoopbackHTTP && ip.IsLoopback() {
		return true
	}
	if opt.AllowLocal {
		return outbound.NotMetadata(ip)
	}
	return outbound.PublicOnly(ip)
}
