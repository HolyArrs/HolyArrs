package doctor

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

// get makes one real call and returns the error, or an error made from the
// answer the way the adapters do (Answer first).
func get(t *testing.T, c *http.Client, url string) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if err := Answer(resp.StatusCode, b); err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return errors.New(resp.Status)
	}
	return nil
}

func closedPort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	return addr
}

func TestOfRealFailures(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/key":
			w.WriteHeader(http.StatusUnauthorized)
		case "/forbidden":
			w.WriteHeader(http.StatusForbidden)
		case "/slow":
			time.Sleep(2 * time.Second)
		case "/ok":
		default:
			http.NotFound(w, r)
		}
	}))
	defer plain.Close()
	secure := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer secure.Close()
	quick := &http.Client{Timeout: 300 * time.Millisecond}
	public := &http.Client{Transport: &http.Transport{DialContext: outbound.PolicyDialer(outbound.PublicOnly).DialContext}}

	cases := []struct {
		name string
		c    *http.Client
		url  string
		want Cause
	}{
		{"refused", http.DefaultClient, "http://" + closedPort(t) + "/", Refused},
		{"timeout", quick, plain.URL + "/slow", Timeout},
		{"self-signed certificate", http.DefaultClient, secure.URL + "/", TLS},
		{"https to an http server", http.DefaultClient, strings.Replace(plain.URL, "http://", "https://", 1) + "/ok", UseHTTP},
		{"http to an https server", http.DefaultClient, strings.Replace(secure.URL, "https://", "http://", 1) + "/", UseHTTPS},
		{"401", http.DefaultClient, plain.URL + "/key", Auth},
		{"403", http.DefaultClient, plain.URL + "/forbidden", Auth},
		{"404", http.DefaultClient, plain.URL + "/sabnzbd/api", NotFound},
		{"local network policy", public, plain.URL + "/ok", Blocked},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := get(t, tc.c, tc.url)
			if got := Of(err); got != tc.want {
				t.Fatalf("Of(%v) = %q, want %q", err, got, tc.want)
			}
			// The same cause comes back from the words alone (a stored last error).
			if got := OfText(err.Error()); got != tc.want {
				t.Fatalf("OfText(%q) = %q, want %q", err.Error(), got, tc.want)
			}
			if Line(tc.want) == "" {
				t.Fatalf("no line for %q", tc.want)
			}
		})
	}
}

func TestOfNameNotFound(t *testing.T) {
	err := &net.DNSError{Err: "no such host", Name: "nas.invalid", IsNotFound: true}
	if got := Of(err); got != DNS {
		t.Fatalf("Of = %q, want %q", got, DNS)
	}
	if got := OfText("qbittorrent app/version: dial tcp: lookup nas.invalid: no such host"); got != DNS {
		t.Fatalf("OfText = %q", got)
	}
}

func TestMarks(t *testing.T) {
	for _, c := range []Cause{WrongApp, Category} {
		err := Mark(c, errors.New("x"))
		if got := Of(err); got != c {
			t.Fatalf("Of(Mark(%q)) = %q", c, got)
		}
		if got := Of(errors.Join(errors.New("outer"), err)); got != c {
			t.Fatalf("wrapped mark lost: %q", got)
		}
	}
}

func TestOfText(t *testing.T) {
	cases := map[string]Cause{
		"qbittorrent app/version: dial tcp 127.0.0.1:4013: connect: connection refused":                                                   Refused,
		"dial tcp 127.0.0.1:4013: connectex: No connection could be made because the target machine actively refused it.":                 Refused,
		"the address answered, but not like SABnzbd":                                                                                      WrongApp,
		"the category does not exist in SABnzbd; add it there first (Config > Categories) (tv)":                                           Category,
		"sabnzbd version: no API at this address":                                                                                         NotFound,
		"qbittorrent app/version: 404 Not Found":                                                                                          NotFound,
		"the server answered 404":                                                                                                         NotFound,
		"the server turned down the token":                                                                                                Auth,
		"Get \"http://x\": context deadline exceeded (Client.Timeout exceeded while awaiting headers)":                                    Timeout,
		"tls: failed to verify certificate: x509: certificate signed by unknown authority":                                                TLS,
		"the Downloader's change feed could not be read: qbittorrent sync/maindata: invalid character '<' looking for beginning of value": WrongApp,
		"the Downloader's change feed could not be read: qbittorrent sync/maindata: unexpected end of JSON input":                         Unknown,
		"the Downloader's change feed could not be read: sabnzbd queue: unexpected EOF":                                                   Unknown,
		"download not found in the Downloader":                                                                                            NotFound,
		"something odd":                                                                                                                   Unknown,
		"":                                                                                                                                Unknown,
	}
	for msg, want := range cases {
		if got := OfText(msg); got != want {
			t.Errorf("OfText(%q) = %q, want %q", msg, got, want)
		}
	}
	for c, line := range causeWords {
		if got := OfText(line); got != c {
			t.Errorf("a stored line %q reads as %q, want %q", line, got, c)
		}
	}
}

func TestExplainUnknownKeepsCallerWords(t *testing.T) {
	if _, _, ok := Explain(errors.New("something odd")); ok {
		t.Fatal("an unknown error must leave the caller's words")
	}
	c, line, ok := Explain(Mark(Category, nil))
	if !ok || c != Category || line != Line(Category) {
		t.Fatalf("Explain = %q %q %v", c, line, ok)
	}
}

func TestAtReadsAHangUp(t *testing.T) {
	probe = func(string, Dial) Cause { return Unknown }
	defer func() { probe = Probe }()
	cases := []struct {
		msg, url string
		want     Cause
	}{
		{"qbittorrent app/version: EOF", "https://127.0.0.1:8080", Unknown},
		{"sabnzbd version: EOF", "http://127.0.0.1:8080", Unknown},
		{"read tcp 127.0.0.1:1->127.0.0.1:2: read: connection reset by peer", "http://nas:9117/api", Unknown},
		{"read tcp 127.0.0.1:1->127.0.0.1:2: read: connection reset by peer", "https://nas:9117/api", Unknown},
		{"wsarecv: An existing connection was forcibly closed by the remote host.", "http://nas:9117/api", Unknown},
		{"qbittorrent app/version: remote error: EOF", "https://127.0.0.1:8080", UseHTTP},
		{"something odd", "https://x", Unknown},
	}
	for _, c := range cases {
		if got := at(OfText(c.msg), c.msg, c.url); got != c.want {
			t.Errorf("At(%q, %q) = %q, want %q", c.msg, c.url, got, c.want)
		}
	}
	if got := at(Unknown, "tls: handshake failure: EOF", "https://x"); got != UseHTTP {
		t.Errorf("an https handshake EOF: %q", got)
	}
	if got := at(Unknown, "tls: handshake failure: EOF", "http://x"); got != Unknown {
		t.Errorf("a handshake EOF on http: %q", got)
	}
	if got := at(Refused, "EOF", "https://x"); got != Refused {
		t.Errorf("a known cause must stay: %q", got)
	}
}

func TestResolverOutageIsNotABadName(t *testing.T) {
	temp := &net.DNSError{Err: "server misbehaving", Name: "sab", IsTemporary: true}
	if c := Of(temp); c == DNS {
		t.Fatalf("temporary resolver failure read as %q", c)
	}
	if c := OfText("lookup sab on 10.0.0.1:53: server misbehaving"); c == DNS {
		t.Fatalf("server misbehaving read as %q", c)
	}
	missing := &net.DNSError{Err: "no such host", Name: "sab", IsNotFound: true}
	if c := Of(missing); c != DNS {
		t.Fatalf("missing name read as %q", c)
	}
}
