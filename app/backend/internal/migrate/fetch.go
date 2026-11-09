package migrate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
)

// Limits bound one read.
type Limits struct {
	// MaxAnswer is the largest API answer read (bytes).
	MaxAnswer int64
	// MaxDepth is the deepest JSON nesting accepted.
	MaxDepth int
	// Timeout bounds each API call.
	Timeout time.Duration
	// MaxFile is the largest single file copied out of a backup.
	MaxFile int64
	// MaxEntries is the most entries a backup zip may hold.
	MaxEntries int
	// MaxRatio is the largest unpacked-to-packed ratio of a zip entry.
	MaxRatio uint64
	// MaxRows is the most rows read from one table or pages from one list.
	MaxRows int
}

// DefaultLimits are used for every zero field.
var DefaultLimits = Limits{
	MaxAnswer:  64 << 20,
	MaxDepth:   64,
	Timeout:    60 * time.Second,
	MaxFile:    1 << 30,
	MaxEntries: 256,
	MaxRatio:   1000,
	MaxRows:    500_000,
}

func (l Limits) orDefault() Limits {
	d := DefaultLimits
	if l.MaxAnswer > 0 {
		d.MaxAnswer = l.MaxAnswer
	}
	if l.MaxDepth > 0 {
		d.MaxDepth = l.MaxDepth
	}
	if l.Timeout > 0 {
		d.Timeout = l.Timeout
	}
	if l.MaxFile > 0 {
		d.MaxFile = l.MaxFile
	}
	if l.MaxEntries > 0 {
		d.MaxEntries = l.MaxEntries
	}
	if l.MaxRatio > 0 {
		d.MaxRatio = l.MaxRatio
	}
	if l.MaxRows > 0 {
		d.MaxRows = l.MaxRows
	}
	return d
}

// apiClient calls one app's API through the Downloaders' network policy:
// public addresses only unless AllowLocal, never link-local or cloud
// metadata addresses, no redirect to another host or scheme.
type apiClient struct {
	base   *url.URL
	h      *http.Client
	lim    Limits
	header func(*http.Request)
	// got counts the bytes read so far, so a paged read can stop before
	// many big pages add up.
	got int64
}

func newAPIClient(src Source, header func(*http.Request)) (*apiClient, error) {
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(src.URL), "/"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" {
		return nil, errors.New("the address must be http:// or https:// with a host, and no user or query")
	}
	lim := src.Limits.orDefault()
	h := downloads.Client(u.String(), src.AllowLocal)
	h.Timeout = lim.Timeout
	return &apiClient{base: u, h: h, lim: lim, header: header}, nil
}

// do calls method path (with query q) and returns the body, capped.
func (c *apiClient) do(ctx context.Context, method, path string, q url.Values, form url.Values) ([]byte, http.Header, int, error) {
	u := *c.base
	u.Path = strings.TrimRight(u.Path, "/") + path
	if q != nil {
		u.RawQuery = q.Encode()
	}
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, nil, 0, ErrBadSource
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	req.Header.Set("Accept", "application/json")
	if c.header != nil {
		c.header(req)
	}
	resp, err := c.h.Do(req)
	if err != nil {
		// Never return the url.Error: its text holds the address and query
		// (SABnzbd's key rides in the query).
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, nil, 0, fmt.Errorf("%w: %s", ErrUnreachable, plainNetErr(err))
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, c.lim.MaxAnswer+1))
	c.got += int64(len(b))
	if err != nil {
		return nil, nil, resp.StatusCode, fmt.Errorf("%w: the answer was cut off", ErrUnreachable)
	}
	if int64(len(b)) > c.lim.MaxAnswer {
		return nil, nil, resp.StatusCode, ErrTooBig
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, nil, resp.StatusCode, ErrBadKey
	case resp.StatusCode >= 300:
		return nil, resp.Header, resp.StatusCode, fmt.Errorf("%w: it answered %d", ErrBadAnswer, resp.StatusCode)
	}
	return b, resp.Header, resp.StatusCode, nil
}

// plainNetErr names a network failure without addresses or secrets.
func plainNetErr(err error) string {
	s := err.Error()
	switch {
	case strings.Contains(s, "refused by the network policy"):
		return "the address is on the local network or not allowed; turn on local network for this read"
	case strings.Contains(s, "another address"):
		return "it sent us to another address"
	case strings.Contains(s, "deadline") || strings.Contains(s, "timeout"):
		return "it took too long"
	case strings.Contains(s, "refused"):
		return "the connection was refused"
	}
	return "the connection failed"
}

// getJSON calls GET path and decodes the JSON answer into v.
func (c *apiClient) getJSON(ctx context.Context, path string, q url.Values, v any) error {
	b, _, _, err := c.do(ctx, http.MethodGet, path, q, nil)
	if err != nil {
		return err
	}
	return decodeJSON(b, c.lim.MaxDepth, v)
}

// decodeJSON decodes b into v after checking its nesting depth.
func decodeJSON(b []byte, maxDepth int, v any) error {
	if err := checkDepth(b, maxDepth); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if err := d.Decode(v); err != nil {
		return fmt.Errorf("%w: not the JSON the app sends", ErrBadAnswer)
	}
	return nil
}

// checkDepth refuses JSON nested deeper than max (a JSON bomb).
func checkDepth(b []byte, max int) error {
	depth, inStr, esc := 0, false, false
	for _, c := range b {
		switch {
		case esc:
			esc = false
		case inStr:
			if c == '\\' {
				esc = true
			} else if c == '"' {
				inStr = false
			}
		case c == '"':
			inStr = true
		case c == '{' || c == '[':
			if depth++; depth > max {
				return fmt.Errorf("%w: nested too deep", ErrBadAnswer)
			}
		case c == '}' || c == ']':
			depth--
		}
	}
	return nil
}

// pagedMax is the most one paged list may read in all: a few answers' worth.
func pagedMax(l Limits) int64 { return 4 * l.MaxAnswer }
