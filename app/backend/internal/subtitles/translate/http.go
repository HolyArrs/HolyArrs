package translate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

// Caps on what a translator may send back.
const (
	maxReply    = 4 << 20
	maxModels   = 500
	callTimeout = 3 * time.Minute
)

// NewHTTP builds the client translators use. It may reach the local network,
// where Ollama and LM Studio often run, but never cloud metadata addresses,
// and it never follows a redirect to another host.
func NewHTTP() *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil // the network rule must see the real address
	t.DialContext = outbound.PolicyDialer(outbound.NotMetadata).DialContext
	return &http.Client{Transport: t, CheckRedirect: sameHost}
}

// shared is the client calls use when they are given none, so a run reuses
// its connection instead of opening a new one for every chunk.
var shared = NewHTTP()

func sameHost(req *http.Request, via []*http.Request) error {
	if len(via) >= 3 || req.URL.Host != via[0].URL.Host || req.URL.Scheme != via[0].URL.Scheme {
		return errors.New("the translator redirected elsewhere; refused")
	}
	return nil
}

// CheckAddress checks a translator address: http or https, a host, no user
// name, and not too long.
func CheckAddress(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || len(raw) > 500 {
		return errors.New("the translator address must be an http or https address without a user name")
	}
	return nil
}

// endpoint joins a base address and a path.
func endpoint(base, p string) string {
	return strings.TrimRight(strings.TrimSpace(base), "/") + p
}

// send makes one call and maps a failed one to an Error. The key is sent in a
// header only and never appears in an error.
func send(ctx context.Context, hc *http.Client, method, addr string, hdr map[string]string, body []byte, timeout time.Duration, secret string) ([]byte, error) {
	if hc == nil {
		hc = shared
	}
	if timeout <= 0 {
		timeout = callTimeout
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(cctx, method, addr, rd)
	if err != nil {
		return nil, &Error{Kind: KindRefused, Msg: "the translator address cannot be used"}
	}
	req.Header.Set("User-Agent", "HolyArrs")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := hc.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err // drop the address so it never leaks
		}
		var ne net.Error
		switch {
		case errors.Is(err, outbound.ErrAddressRefused):
			return nil, &Error{Kind: KindRefused, Msg: "the translator address is not allowed"}
		case errors.As(err, &ne) && ne.Timeout(), errors.Is(err, context.DeadlineExceeded):
			return nil, &Error{Kind: KindCut, Msg: "the translator did not answer in time"}
		}
		return nil, &Error{Kind: KindDown, Msg: scrub(err.Error(), secret)}
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(io.LimitReader(resp.Body, maxReply+1))
	if err != nil {
		if errors.Is(cctx.Err(), context.DeadlineExceeded) {
			return nil, &Error{Kind: KindCut, Msg: "the translator did not answer in time"}
		}
		return nil, &Error{Kind: KindDown, Msg: "reading the answer failed"}
	}
	if len(out) > maxReply {
		return nil, &Error{Kind: KindUnreadable, Msg: "the answer is too big"}
	}
	switch code := resp.StatusCode; {
	case code >= 200 && code < 300:
		return out, nil
	case code == http.StatusTooManyRequests:
		e := &Error{Kind: KindBusy, Msg: "the translator asked HolyArrs to slow down"}
		if s, err := strconv.Atoi(strings.TrimSpace(resp.Header.Get("Retry-After"))); err == nil && s > 0 {
			e.RetryAfter = time.Duration(min(s, 300)) * time.Second
		}
		return nil, e
	case code == http.StatusUnauthorized || code == http.StatusForbidden || code == http.StatusPaymentRequired:
		return nil, &Error{Kind: KindRefused, Msg: fmt.Sprintf("the translator refused the key (%d)", code)}
	case code == http.StatusRequestEntityTooLarge:
		return nil, &Error{Kind: KindCut, Msg: "the translator found the request too big"}
	case code >= 500:
		return nil, &Error{Kind: KindBusy, Msg: fmt.Sprintf("the translator failed (%d)", code)}
	default:
		msg := fmt.Sprintf("the translator refused the call (%d)", code)
		if why := reason(out, secret); why != "" {
			msg += ": " + why
		}
		return nil, &Error{Kind: KindRefused, Msg: msg}
	}
}

// reason reads a short error message from an answer, such as an unknown
// model, with the key taken out.
func reason(body []byte, secret string) string {
	var v struct {
		Error any `json:"error"`
	}
	if decodeLoose(body, &v) != nil || v.Error == nil {
		return ""
	}
	msg := ""
	switch e := v.Error.(type) {
	case string:
		msg = e
	case map[string]any:
		msg, _ = e["message"].(string)
	}
	msg = strings.Join(strings.Fields(scrub(msg, secret)), " ")
	if len(msg) > 200 {
		msg = msg[:200]
	}
	return msg
}

// scrub takes the key, and anything that looks like part of one, out of msg.
func scrub(msg, secret string) string {
	if len(secret) >= 3 {
		msg = strings.ReplaceAll(msg, secret, "[hidden]")
		msg = strings.ReplaceAll(msg, url.QueryEscape(secret), "[hidden]")
	}
	words := strings.Fields(msg)
	for i, w := range words {
		if strings.HasPrefix(strings.ToLower(strings.Trim(w, `"'.,:`)), "sk-") {
			words[i] = "[hidden]"
		}
	}
	return strings.Join(words, " ")
}
