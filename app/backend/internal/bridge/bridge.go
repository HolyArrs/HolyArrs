// Package bridge connects HolyArrs to a Jackett or Prowlarr the person
// runs (#96), so the sites set up there become Sources in one step and
// stay in step. Jackett: HolyArrs reads its configured sites and keeps one
// Torznab Source per site. Prowlarr: HolyArrs adds itself to Prowlarr as an
// app, and Prowlarr's own app sync (roadmap #85, internal/compat) writes
// the Sources. Facts about both come from their manuals
// (research/upstream/{jackett,prowlarr}/manual).
package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Kinds of bridge.
const (
	KindJackett  = "jackett"
	KindProwlarr = "prowlarr"
)

// Error is a failed call to a bridge, in plain words; it never holds the key.
type Error struct {
	Words string
}

func (e *Error) Error() string { return e.Words }

func fail(format string, args ...any) error { return &Error{Words: fmt.Sprintf(format, args...)} }

// Words is what went wrong, for the person.
func Words(err error) string {
	var be *Error
	if errors.As(err, &be) {
		return be.Words
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "It took too long to answer."
	}
	return "Something went wrong talking to it."
}

// client talks to a bridge on the home network: local addresses are fine
// (that is where these run), a cloud's metadata address never is, and
// redirects are not followed (a key must not go to another host).
func client(timeout time.Duration) *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil
	t.DialContext = outbound.PolicyDialer(outbound.NotMetadata).DialContext
	return &http.Client{Timeout: timeout, Transport: t, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// CleanURL checks a bridge address: http or https, a host, no name or
// password, no ? part; the trailing slash is dropped.
func CleanURL(raw string) (string, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fail("The address must start with http:// or https://, with no name, password or ? part.")
	}
	return raw, nil
}

// call does one request and reads at most limit bytes of the answer.
func call(ctx context.Context, hc *http.Client, method, target string, header map[string]string, body any, limit int64) (int, []byte, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, rd)
	if err != nil {
		return 0, nil, fail("The address can't be used.")
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json, application/xml;q=0.9, */*;q=0.5")
	resp, err := hc.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return 0, nil, ctx.Err()
		}
		if errors.Is(err, outbound.ErrAddressRefused) {
			return 0, nil, fail("That address can't be used: it is a cloud's internal address.")
		}
		return 0, nil, fail("HolyArrs could not reach it at that address. Check the address and that it is running.")
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return resp.StatusCode, nil, fail("HolyArrs could not read its answer.")
	}
	return resp.StatusCode, b, nil
}
