// Package doctor names why a linked app (a Downloader, a Source, a media
// server) cannot be reached: one cause, one short line with the fix. Test
// buttons and health rows use the same lines for every kind.
package doctor

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"strings"
	"syscall"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

// Cause is why a connection failed.
type Cause string

// The causes, as the API sends them.
const (
	Unknown  Cause = ""
	DNS      Cause = "name_not_found"
	Refused  Cause = "refused"
	Timeout  Cause = "timeout"
	TLS      Cause = "certificate"
	UseHTTP  Cause = "use_http"
	UseHTTPS Cause = "use_https"
	Auth     Cause = "auth"
	NotFound Cause = "wrong_path"
	WrongApp Cause = "wrong_app"
	Category Cause = "category_missing"
	Blocked  Cause = "local_network"
)

// causeWords is each cause's line: a short title, then the fix.
var causeWords = map[Cause]string{
	DNS:      "Name not found. Check the host name in the address.",
	Refused:  "Nothing answers on that port. Check the port, and that the app is running.",
	Timeout:  "No answer in time. Check the address, and that no firewall is in the way.",
	TLS:      "Certificate not trusted. Use the name on its certificate, or http:// if it has none.",
	UseHTTP:  "It speaks http, not https. Start the address with http://.",
	UseHTTPS: "It speaks https, not http. Start the address with https://.",
	Auth:     "Key or password refused. Check them, then test again.",
	NotFound: "Nothing at that path. Check the path after the port, for example /sabnzbd.",
	WrongApp: "Another app answered. Check the port: it belongs to something else.",
	Category: "Category missing. Add it in the download app, or pick another one here.",
	Blocked:  "Blocked: local network. Turn on \"Reach the local network\" if it runs at home.",
}

// Line is the cause's line, "" for Unknown.
func Line(c Cause) string { return causeWords[c] }

// Fault is an error marked with its cause by the code that knows it best.
type Fault struct {
	Cause Cause
	Err   error
}

func (f *Fault) Error() string {
	if f.Err == nil {
		return causeWords[f.Cause]
	}
	return f.Err.Error()
}

func (f *Fault) Unwrap() error { return f.Err }

// Mark ties a cause to err. A nil err gets the cause's line as its text.
func Mark(c Cause, err error) error { return &Fault{Cause: c, Err: err} }

// Answer marks an HTTP answer whose status or body names a cause, and is nil
// otherwise. Callers keep their own errors for the rest.
func Answer(code int, body []byte) error {
	switch {
	case code == 400 && strings.Contains(string(body), "HTTP request to an HTTPS server"):
		return Mark(UseHTTPS, nil)
	case code == 401 || code == 403:
		return Mark(Auth, nil)
	case code == 404:
		return Mark(NotFound, nil)
	}
	return nil
}

// Of names the cause of err: a mark first, then the network error itself,
// then its words (a stored last error is only text).
func Of(err error) Cause {
	if err == nil {
		return Unknown
	}
	var f *Fault
	if errors.As(err, &f) && f.Cause != Unknown {
		return f.Cause
	}
	if errors.Is(err, outbound.ErrAddressRefused) {
		return Blocked
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && !dnsErr.IsTimeout {
		return DNS
	}
	var rh tls.RecordHeaderError
	if errors.As(err, &rh) {
		return UseHTTP
	}
	if isCert(err) {
		return TLS
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return Refused
	}
	var ne net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &ne) && ne.Timeout() {
		return Timeout
	}
	return OfText(err.Error())
}

func isCert(err error) bool {
	var ua x509.UnknownAuthorityError
	var he x509.HostnameError
	var ci x509.CertificateInvalidError
	var cv *tls.CertificateVerificationError
	return errors.As(err, &ua) || errors.As(err, &he) || errors.As(err, &ci) || errors.As(err, &cv)
}

// textCauses are the words of errors that only reach here as text, in the
// order they are tried.
var textCauses = []struct {
	c     Cause
	words []string
}{
	{Blocked, []string{"address refused by the network policy", "on the local network"}},
	{DNS, []string{"no such host", "server misbehaving", "could not look up"}},
	{UseHTTP, []string{"server gave http response to https client", "first record does not look like a tls handshake"}},
	{UseHTTPS, []string{"http request to an https server"}},
	{TLS, []string{"x509:", "certificate", "tls: "}},
	{Refused, []string{"connection refused", "actively refused"}},
	{Timeout, []string{"timeout", "deadline exceeded", "did not answer within", "took too long"}},
	{Category, []string{"category does not exist", "category missing"}},
	{WrongApp, []string{"answered, but not like", "not with a feed", "not valid xml", "another app answered"}},
	{Auth, []string{"401 unauthorized", "403 forbidden", "answered 401", "answered 403", "turned down the key", "turned down the token", "wants a sign-in", "sign-in refused"}},
	{NotFound, []string{"404 not found", "answered 404", "no api at this address", "not at this path", "nothing at that path"}},
}

// OfText names the cause from an error's words alone.
func OfText(msg string) Cause {
	m := strings.ToLower(msg)
	if m == "" {
		return Unknown
	}
	for c, line := range causeWords {
		if strings.Contains(msg, line) {
			return c
		}
	}
	for _, tc := range textCauses {
		for _, w := range tc.words {
			if strings.Contains(m, w) {
				return tc.c
			}
		}
	}
	return Unknown
}

// Explain is the cause and its line for err; ok is false when the cause is
// unknown, so the caller keeps its own words.
func Explain(err error) (Cause, string, bool) {
	c := Of(err)
	return c, causeWords[c], c != Unknown
}

// At refines a cause with the address that was called. A connection closed
// without any answer is a protocol mismatch: an https:// address on a plain
// http port (qBittorrent just hangs up), or http on a port that speaks
// something else.
func At(c Cause, msg, rawURL string) Cause {
	if c != Unknown {
		return c
	}
	m := strings.ToLower(msg)
	hungUp := strings.HasSuffix(strings.TrimSpace(m), "eof")
	for _, w := range []string{"connection reset", "forcibly closed", "connection was aborted", "broken pipe"} {
		hungUp = hungUp || strings.Contains(m, w)
	}
	if !hungUp {
		return Unknown
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(rawURL)), "https://") {
		return UseHTTP
	}
	return WrongApp
}

// ExplainAt is Explain for a call to rawURL.
func ExplainAt(err error, rawURL string) (Cause, string, bool) {
	if err == nil {
		return Unknown, "", false
	}
	c := At(Of(err), err.Error(), rawURL)
	return c, causeWords[c], c != Unknown
}
