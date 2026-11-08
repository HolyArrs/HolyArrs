package sources

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

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

// The kinds of bridge.
const (
	BridgeKindJackett  = "jackett"
	BridgeKindProwlarr = "prowlarr"
)

// BridgeError is a failed call to a bridge, worded for the person. It never holds
// the key.
type BridgeError struct {
	Words string
	// Detail is what the bridge itself said; it goes to the log only, never
	// into a reply, since the remote side picks it.
	Detail string
}

// Error returns the plain words of the failure.
func (e *BridgeError) Error() string { return e.Words }

// BridgeWords returns what went wrong, worded for the person.
func BridgeWords(err error) string {
	var be *BridgeError
	if errors.As(err, &be) {
		return be.Words
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "It took too long to answer."
	}
	return "Couldn't connect."
}

// BridgeCleanURL checks a bridge address and drops its trailing slash. It must be
// http or https with a host, and carry no user, password, query or fragment.
func BridgeCleanURL(raw string) (string, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fail("The address must start with http:// or https://, with no name, password or ? part.")
	}
	return raw, nil
}

func fail(format string, args ...any) error { return &BridgeError{Words: fmt.Sprintf(format, args...)} }

// client builds an HTTP client for a bridge on the home network. Local
// addresses are allowed because bridges run there, but a cloud metadata
// address is refused. Redirects are not followed so a key never goes to
// another host.
func client(timeout time.Duration) *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil
	t.DialContext = outbound.PolicyDialer(outbound.NotMetadata).DialContext
	return &http.Client{Timeout: timeout, Transport: t, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// call sends one request and reads at most limit bytes of the answer, so a
// huge or hostile reply cannot fill memory.
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
