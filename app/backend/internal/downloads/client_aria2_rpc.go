package downloads

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

// aria2AnswerMax caps an answer. A large aria2 lists many thousands of jobs,
// each with its files and tracker list, in one call.
var aria2AnswerMax int64 = 64 << 20

// aria2BodyLimit is aria2's default --rpc-max-request-size. aria2 drops the
// connection without an answer when a call is larger than its limit.
const aria2BodyLimit = 2 << 20

var errNotAria2 = errors.New("this address is not aria2's JSON-RPC; the address usually ends in /jsonrpc")

// Aria2Path returns the address path HolyArrs uses for aria2's JSON-RPC.
// No path, or "/", becomes "/jsonrpc". A path ending in "/rpc" (the XML-RPC
// address other apps use) is switched to the same place ending in "/jsonrpc".
// Any other path is kept as written.
func Aria2Path(p string) string {
	t := strings.TrimRight(p, "/")
	switch {
	case t == "":
		return "/jsonrpc"
	case strings.HasSuffix(t, "/rpc"):
		return strings.TrimSuffix(t, "/rpc") + "/jsonrpc"
	case t == "rpc":
		return "/jsonrpc"
	}
	return p
}

type aria2Req struct {
	JSONRPC string `json:"jsonrpc"`
	ID      string `json:"id"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
}

type aria2Resp struct {
	ID     string          `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *a2Fault        `json:"error"`
}

// a2Fault is a call aria2 refused. aria2 always answers code 1; only the
// message tells the cases apart.
type a2Fault struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Error implements error.
func (f *a2Fault) Error() string {
	return "aria2 refused the call: " + redactURLs(cleanDetail(f.Message))
}

// a2Call is one call inside a system.multicall.
type a2Call struct {
	Method string
	Args   []any
}

// aria2TooLarge is a call refused for its size before aria2 took it.
type aria2TooLarge struct{ error }

// Unwrap gives the cause.
func (e aria2TooLarge) Unwrap() error { return e.error }

// faultErr maps aria2's fault messages onto the shared errors.
func faultErr(f *a2Fault) error {
	m := f.Message
	switch {
	case m == "Unauthorized":
		return ErrAuth
	case strings.HasPrefix(m, "GID ") && strings.HasSuffix(m, " is not found"), strings.HasPrefix(m, "Invalid GID"):
		return ErrNotFound
	case strings.HasPrefix(m, "No such method"):
		return fmt.Errorf("aria2 does not offer a call HolyArrs needs (%s); check that the address is aria2's JSON-RPC, which usually ends in /jsonrpc", cleanDetail(m))
	}
	return f
}

// faultSays reports a refused call whose message holds s.
func faultSays(err error, s string) bool {
	var f *a2Fault
	return errors.As(err, &f) && strings.Contains(f.Message, s)
}

// a2Num reads one of aria2's numbers, which all come as strings. A bad one reads as 0.
func a2Num(s string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// params puts the secret first, only when there is one.
func (a *Aria2) params(extra ...any) []any {
	p := make([]any, 0, len(extra)+1)
	if a.secret != "" {
		p = append(p, aria2TokenParam(a.secret))
	}
	return append(p, extra...)
}

// call sends one call and decodes its result into out, when out is not nil.
func (a *Aria2) call(ctx context.Context, method string, out any, args ...any) error {
	raw, err := a.post(ctx, aria2Req{JSONRPC: "2.0", ID: "holyarrs", Method: method, Params: a.params(args...)})
	if err != nil || out == nil {
		return err
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return errNotAria2
	}
	return nil
}

// multicall sends calls in one system.multicall. Each answer is the call's
// result, or an error in the same place for a call aria2 refused.
func (a *Aria2) multicall(ctx context.Context, calls []a2Call) ([]json.RawMessage, []error, error) {
	list := make([]any, len(calls))
	for i, c := range calls {
		list[i] = map[string]any{"methodName": c.Method, "params": a.params(c.Args...)}
	}
	raw, err := a.post(ctx, aria2Req{JSONRPC: "2.0", ID: "holyarrs", Method: "system.multicall", Params: []any{list}})
	if err != nil {
		return nil, nil, err
	}
	var entries []json.RawMessage
	if json.Unmarshal(raw, &entries) != nil || len(entries) != len(calls) {
		return nil, nil, errNotAria2
	}
	answers, errs := make([]json.RawMessage, len(calls)), make([]error, len(calls))
	for i, e := range entries {
		e = bytes.TrimSpace(e)
		switch {
		case len(e) > 0 && e[0] == '[':
			var one []json.RawMessage
			if json.Unmarshal(e, &one) != nil || len(one) != 1 {
				return nil, nil, errNotAria2
			}
			answers[i] = one[0]
		case len(e) > 0 && e[0] == '{':
			var f a2Fault
			if json.Unmarshal(e, &f) != nil {
				return nil, nil, errNotAria2
			}
			errs[i] = faultErr(&f)
		default:
			return nil, nil, errNotAria2
		}
	}
	return answers, errs, nil
}

// post sends one JSON-RPC request and returns its result.
func (a *Aria2) post(ctx context.Context, r aria2Req) (json.RawMessage, error) {
	body, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("aria2: invalid address")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, outbound.ErrAddressRefused) {
			return nil, outbound.ErrAddressRefused
		}
		if len(body) > aria2BodyLimit {
			return nil, aria2TooLarge{fmt.Errorf("%w: aria2 dropped a call of %d bytes; raise aria2's --rpc-max-request-size (default 2M)", ErrTooLarge, len(body))}
		}
		return nil, errDownloaderUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, ErrAuth
	}
	if resp.StatusCode == http.StatusRequestEntityTooLarge {
		return nil, aria2TooLarge{fmt.Errorf("%w: the call is over a size limit; raise aria2's --rpc-max-request-size (default 2M) or the web server's body size limit", ErrTooLarge)}
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, aria2AnswerMax+1))
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errDownloaderUnavailable
	}
	if int64(len(b)) > aria2AnswerMax {
		return nil, ErrTooLarge
	}
	var out aria2Resp
	if json.Unmarshal(b, &out) != nil || out.Result == nil && out.Error == nil {
		ok := resp.StatusCode >= 200 && resp.StatusCode <= 299
		if ok || resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed {
			return nil, errNotAria2
		}
		return nil, adapterHTTPError{resp.StatusCode}
	}
	if out.Error != nil {
		return nil, faultErr(out.Error)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, adapterHTTPError{resp.StatusCode}
	}
	return out.Result, nil
}

// aria2TokenParam is the first parameter aria2 wants when rpc-secret is set.
func aria2TokenParam(s string) string { return "token:" + s }
