package plugins

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (e *Engine) fetch(ctx context.Context, rel string) ([]byte, error) {
	body, err := e.fetchBody(ctx, rel, maxListSize)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	data, err := io.ReadAll(io.LimitReader(body, maxListSize+1))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFetch, err)
	}
	if len(data) > maxListSize {
		return nil, fmt.Errorf("%w: %s is too large", ErrFetch, rel)
	}
	return data, nil
}

// fetchBody GETs rel below ListURL (or rel itself when it is a full https
// address) over https; certificates are always
// verified. The caller closes the body.
func (e *Engine) fetchBody(ctx context.Context, rel string, max int64) (io.ReadCloser, error) {
	base, err := url.Parse(e.ListURL)
	if err != nil || base.Scheme != "https" || base.Host == "" {
		return nil, fmt.Errorf("%w: list address %q is not https", ErrFetch, e.ListURL)
	}
	if !strings.HasSuffix(base.Path, "/") {
		base.Path += "/"
	}
	u, err := base.Parse(rel)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFetch, err)
	}
	if u.Scheme != "https" || u.Host == "" {
		return nil, fmt.Errorf("%w: %s is not https", ErrFetch, u.Redacted())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFetch, err)
	}
	c := http.Client{Timeout: 5 * time.Minute}
	if e.HTTP != nil {
		c = *e.HTTP
	}
	c.CheckRedirect = RedirectGuard(u.String())
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFetch, err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("%w: %s answered %d", ErrFetch, rel, resp.StatusCode)
	}
	if resp.ContentLength > max {
		resp.Body.Close()
		return nil, fmt.Errorf("%w: %s is too large", ErrFetch, rel)
	}
	return resp.Body, nil
}

// RedirectGuard refuses any redirect that leaves https or the host of
// listURL. Every list client uses it.
func RedirectGuard(listURL string) func(*http.Request, []*http.Request) error {
	host := ""
	if u, err := url.Parse(listURL); err == nil {
		host = u.Host
	}
	return func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" || req.URL.Host != host {
			return fmt.Errorf("%w: redirect to %s refused", ErrFetch, req.URL.Redacted())
		}
		if len(via) >= 10 {
			return fmt.Errorf("%w: too many redirects", ErrFetch)
		}
		return nil
	}
}
