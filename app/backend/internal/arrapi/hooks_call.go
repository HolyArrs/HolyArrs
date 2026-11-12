package arrapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"syscall"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

// hookHTTP calls hooks. It may reach the local network, where Pulsarr
// nearly always runs next to HolyArrs, but never link-local or cloud
// metadata addresses: the check runs on every address after DNS. No proxy
// is used, so the check sees the real address, and redirects are never
// followed, so a secret header never goes on to another server. Each call
// sets its own deadline.
var hookHTTP = func() *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil
	d := outbound.PolicyDialer(outbound.NotMetadata)
	policy := d.Control
	d.Control = func(network, address string, c syscall.RawConn) error {
		if err := policy(network, address, c); err != nil {
			return err
		}
		if host, port, err := net.SplitHostPort(address); err == nil && selfAddr(net.ParseIP(host), port) {
			return errSelf
		}
		return nil
	}
	t.DialContext = d.DialContext
	return &http.Client{Transport: t, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}()

// errSelf is the dial error for an address that is HolyArrs itself.
var errSelf = errors.New("the address is HolyArrs itself")

// hookError is a failed call. Its text is safe to show and log: it never
// holds the address or a header. again marks a failure worth another try
// (no answer, or a server error); a refusal such as 400 or 401 is not.
type hookError struct {
	text  string
	again bool
}

func (e *hookError) Error() string { return e.text }

// worthRetry reports whether err is worth another try.
func worthRetry(err error) bool {
	var he *hookError
	return errors.As(err, &he) && he.again
}

// eventName is the name HolyArrs gives itself in the events it sends.
const eventName = AppName

// testEvent is the test call's body. It carries no title on purpose:
// Pulsarr reads any body with a movie in it as "this movie is ready".
func testEvent() []byte {
	b, _ := json.Marshal(map[string]string{"eventType": "Test", "instanceName": eventName})
	return b
}

// callHook sends body to a hook once, with its method, extra headers and
// sign-in. Any 2xx answer is success.
func callHook(ctx context.Context, method string, sec hookSecret, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, method, sec.URL, bytes.NewReader(body))
	if err != nil {
		return &hookError{text: "the address is not valid"}
	}
	for _, hd := range sec.Headers {
		req.Header.Add(hd.Key, hd.Value)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "HolyArrs")
	req.Header.Set(outbound.SelfCallHeader, "hook")
	if sec.Username != "" || sec.Password != "" {
		req.SetBasicAuth(sec.Username, sec.Password)
	}
	resp, err := hookHTTP.Do(req)
	if err != nil {
		// The address may hold a token, so the error never keeps it.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		switch {
		case errors.Is(err, errSelf):
			return &hookError{text: "the address points back at HolyArrs itself"}
		case errors.Is(err, outbound.ErrAddressRefused):
			return &hookError{text: "the address points to a network HolyArrs may not reach"}
		case ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded):
			return &hookError{text: "the address did not answer in time", again: true}
		}
		return &hookError{text: "the address did not answer", again: true}
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	_ = resp.Body.Close()
	switch c := resp.StatusCode; {
	case c/100 == 2:
		return nil
	case c/100 == 3:
		return &hookError{text: fmt.Sprintf("the address answered %d, a redirect, which HolyArrs does not follow", c)}
	case c >= 500:
		return &hookError{text: fmt.Sprintf("the address answered %d", c), again: true}
	default:
		return &hookError{text: fmt.Sprintf("the address answered %d", c)}
	}
}
