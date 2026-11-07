package downloader

import (
	"context"
	"errors"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"io"
	"net/http"
	"net/url"
)

// fetchRelease uses the Source's network permission and no Downloader
// authentication. Handing an arbitrary URL to an app would bypass this policy.
func fetchRelease(ctx context.Context, raw string, allowLocal bool, limit int64) ([]byte, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Fragment != "" {
		return nil, errors.New("the Release file needs an http or https address without a sign-in")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, errors.New("the Release file address is invalid")
	}
	h := Client(raw, allowLocal)
	defer h.CloseIdleConnections()
	resp, err := h.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, outbound.ErrAddressRefused) {
			return nil, errLinkLocal // final: not fetched again on every retry (#152)
		}
		return nil, errors.New("the Release file could not be read from its Source")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, errors.New("the Source refused the Release file")
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("the Release file could not be read completely")
	}
	if len(b) == 0 || int64(len(b)) > limit {
		return nil, errors.New("the Release file is empty or too large")
	}
	return b, nil
}
