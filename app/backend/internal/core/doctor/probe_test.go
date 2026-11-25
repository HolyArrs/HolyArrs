package doctor

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

// hangUp listens on 127.0.0.1 and runs serve on every connection.
func hangUp(t *testing.T, serve func(net.Conn)) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); serve(c) }()
		}
	}()
	return ln.Addr().String()
}

func call(t *testing.T, url string) error {
	t.Helper()
	c := &http.Client{Timeout: 3 * time.Second}
	return get(t, c, url)
}

func TestPlainHTTPToHTTPSPort(t *testing.T) {
	// nginx's answer to plain http on its https port.
	nginx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Server", "nginx")
		w.WriteHeader(400)
		io.WriteString(w, "<html><center>The plain HTTP request was sent to HTTPS port</center></html>")
	}))
	defer nginx.Close()
	if err := call(t, nginx.URL); Of(err) != UseHTTPS {
		t.Errorf("nginx 400: %v -> %q", err, Of(err))
	}
	// A TLS port that answers plain http with a TLS alert record.
	alert := hangUp(t, func(c net.Conn) {
		bufio.NewReader(c).ReadString('\n')
		c.Write([]byte{0x15, 0x03, 0x01, 0x00, 0x02, 0x02, 0x46})
	})
	err := call(t, "http://"+alert+"/api")
	if got := at(Of(err), err.Error(), "http://"+alert+"/api"); got != UseHTTPS {
		t.Errorf("TLS alert on a plain read: %v -> %q", err, got)
	}
	// A TLS port that hangs up on plain http: only asking again finds it.
	tlsSrv := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	tlsSrv.Config.ErrorLog = nil
	tlsSrv.StartTLS()
	defer tlsSrv.Close()
	plain := "http://" + strings.TrimPrefix(tlsSrv.URL, "https://")
	if got := at(Unknown, "read: connection reset by peer", plain); got != UseHTTPS {
		t.Errorf("reset from a TLS port: %q", got)
	}
}

func TestHTTPSToPlainPort(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	defer plain.Close()
	tlsURL := "https://" + strings.TrimPrefix(plain.URL, "http://")
	for _, msg := range []string{"qbittorrent app/version: EOF", "the server did not answer", "did not answer"} {
		if got := at(OfText(msg), msg, tlsURL); got != UseHTTP {
			t.Errorf("%q on a plain port: %q", msg, got)
		}
	}
	// A TLS port that says so in plain text (nginx, Cloudflare): no guess.
	tlsPort := hangUp(t, func(c net.Conn) {
		_, _ = c.Read(make([]byte, 512))
		_, _ = io.WriteString(c, "HTTP/1.1 400 Bad Request\r\nServer: cloudflare\r\nContent-Type: text/html\r\n\r\n<html><head><title>400 The plain HTTP request was sent to HTTPS port</title></head></html>")
	})
	if got := at(Unknown, "EOF", "https://"+tlsPort); got != Unknown {
		t.Errorf("a TLS port's plain-text answer: %q", got)
	}
	// A plain port that says nothing and closes: no guess.
	silent := hangUp(t, func(net.Conn) {})
	if got := at(Unknown, "EOF", "https://"+silent); got != Unknown {
		t.Errorf("silent hang-up: %q", got)
	}
	if got := at(Unknown, "EOF", "http://"+silent); got != Unknown {
		t.Errorf("silent hang-up on http: %q", got)
	}
}

func TestOtherAppAnswered(t *testing.T) {
	h := func(kv ...string) http.Header {
		x := http.Header{}
		for i := 0; i+1 < len(kv); i += 2 {
			x.Set(kv[i], kv[i+1])
		}
		return x
	}
	cases := []struct {
		name string
		h    http.Header
		body string
		own  []string
		want Cause
	}{
		{"sonarr on the qbit port", h(), `<html><head><title>Sonarr</title>`, []string{"qbittorrent"}, WrongApp},
		{"jellyfin on the plex port", h("Server", "Kestrel", "X-Response-Time-ms", "1"), `{"Name":"Jellyfin Server"}`, []string{"plex"}, WrongApp},
		{"plex on the jellyfin port", h("X-Plex-Protocol", "1.0"), `<html><head><title>Unauthorized</title>`, []string{"jellyfin", "emby"}, WrongApp},
		{"qbit itself", h(), `Forbidden`, []string{"qbittorrent"}, Unknown},
		{"sab itself", h("Server", "SABnzbd"), `API Key Required`, []string{"sabnzbd"}, Unknown},
		{"prowlarr for a source", h(), `<title>Prowlarr</title>`, []string{"prowlarr", "jackett"}, Unknown},
		{"no name", h("Server", "nginx"), `Not Found`, []string{"qbittorrent"}, Unknown},
		{"a word, not plex", h(), `a complex error`, []string{"jellyfin"}, Unknown},
	}
	for _, c := range cases {
		if got := Of(Other(c.h, []byte(c.body), c.own...)); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

// at is At with a dial that lets every address through.
func at(c Cause, msg, rawURL string) Cause {
	var d net.Dialer
	return At(c, msg, rawURL, d.DialContext)
}

func TestProbeKeepsThePolicy(t *testing.T) {
	var hits atomic.Int32
	tlsSrv := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	tlsSrv.Config.ConnState = func(net.Conn, http.ConnState) { hits.Add(1) }
	tlsSrv.StartTLS()
	defer tlsSrv.Close()
	plain := "http://" + strings.TrimPrefix(tlsSrv.URL, "https://")
	// The policy refuses this machine: the second look never connects.
	refuse := outbound.PolicyDialer(outbound.PublicOnly).DialContext
	if got := At(Unknown, "read: connection reset by peer", plain, refuse); got != Unknown {
		t.Errorf("policy-refused probe: %q", got)
	}
	// No dial (a proxy carries the calls): no second look at all.
	if got := At(Unknown, "read: connection reset by peer", plain, nil); got != Unknown {
		t.Errorf("probe without a dial: %q", got)
	}
	// An error the policy gave is never looked at again.
	open := (&net.Dialer{}).DialContext
	if got := At(Unknown, "dial tcp: "+outbound.ErrAddressRefused.Error(), plain, open); got != Unknown {
		t.Errorf("probe after a policy refusal: %q", got)
	}
	time.Sleep(100 * time.Millisecond)
	if n := hits.Load(); n != 0 {
		t.Errorf("the probe reached the server %d times", n)
	}
}
