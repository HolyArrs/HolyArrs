package metadata

import (
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"testing"
)

// TestServerTransportHTTP2Pool: with a custom dialer (the address policy)
// the server transport still speaks HTTP/2, and the second request reuses
// the first one's connection (one TLS setup, not two).
func TestServerTransportHTTP2Pool(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, r.Proto)
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()
	d := &net.Dialer{}
	tr := NewServerTransport(d.DialContext)
	tr.TLSClientConfig = srv.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	tr.TLSClientConfig.NextProtos = []string{"h2", "http/1.1"}
	cl := &http.Client{Transport: tr}
	var reused []bool
	handshakes := 0
	for i := 0; i < 3; i++ {
		trace := &httptrace.ClientTrace{
			GotConn:          func(c httptrace.GotConnInfo) { reused = append(reused, c.Reused) },
			TLSHandshakeDone: func(tls.ConnectionState, error) { handshakes++ },
		}
		req, _ := http.NewRequestWithContext(httptrace.WithClientTrace(t.Context(), trace), http.MethodGet, srv.URL, nil)
		resp, err := cl.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.ProtoMajor != 2 || string(b) != "HTTP/2.0" {
			t.Fatalf("request %d spoke %s (server saw %s), want HTTP/2", i, resp.Proto, b)
		}
	}
	if handshakes != 1 || len(reused) != 3 || reused[0] || !reused[1] || !reused[2] {
		t.Fatalf("handshakes %d, reused %v: want one TLS setup and the connection kept", handshakes, reused)
	}
}

// TestHostRoute: only the named host goes through Via.
func TestHostRoute(t *testing.T) {
	var via, rest int
	h := HostRoute{Host: "api.example", Via: rtFunc(func(*http.Request) (*http.Response, error) { via++; return nil, io.EOF }),
		Rest: rtFunc(func(*http.Request) (*http.Response, error) { rest++; return nil, io.EOF })}
	for _, u := range []string{"https://api.example/v1/img/x", "https://image.tmdb.org/t/p/w500/a.jpg", "https://api.example.evil/x"} {
		req, _ := http.NewRequest(http.MethodGet, u, nil)
		_, _ = h.RoundTrip(req)
	}
	if via != 1 || rest != 2 {
		t.Fatalf("via %d rest %d", via, rest)
	}
}

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
