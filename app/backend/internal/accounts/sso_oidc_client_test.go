package accounts

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestOIDCHTTPClientRefusesRedirectsAndLargeBodies(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ok", http.StatusFound)
	})
	mux.HandleFunc("/large", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, strings.Repeat("x", oidcBodyLimit+1))
	})
	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "ok") })
	srv := httptest.NewServer(mux)
	defer srv.Close()

	issuer, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	c := newOIDCHTTPClient(OIDCOptions{AllowLoopbackHTTP: true}, issuer)
	resp, err := c.Get(srv.URL + "/redirect")
	if err == nil {
		resp.Body.Close()
		t.Fatal("redirect followed")
	}
	if !errors.Is(err, errOIDCRedirect) {
		t.Fatalf("redirect error = %v", err)
	}
	resp, err = c.Get(srv.URL + "/large")
	if resp != nil {
		resp.Body.Close()
	}
	if !errors.Is(err, errOIDCBodyLarge) {
		t.Fatalf("large body error = %v", err)
	}
}

func TestOIDCHTTPClientSuppressesRepeatedProviderFailures(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	issuer, _ := url.Parse(srv.URL)
	c := newOIDCHTTPClient(OIDCOptions{ClientID: "breaker-test", AllowLoopbackHTTP: true}, issuer)
	for range 3 {
		resp, err := c.Get(srv.URL + "/token")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	resp, err := c.Get(srv.URL + "/token")
	if resp != nil {
		resp.Body.Close()
	}
	if !errors.Is(err, errOIDCSuppressed) {
		t.Fatalf("suppressed error = %v", err)
	}
}

func TestOIDCAddressPolicy(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		opts OIDCOptions
		ip   string
		want bool
	}{
		{"public", OIDCOptions{}, "8.8.8.8", true},
		{"private default", OIDCOptions{}, "10.0.0.4", false},
		{"private opted in", OIDCOptions{AllowLocal: true}, "10.0.0.4", true},
		{"metadata", OIDCOptions{AllowLocal: true}, "169.254.169.254", false},
		{"multicast", OIDCOptions{AllowLocal: true}, "224.0.0.1", false},
		{"unspecified", OIDCOptions{AllowLocal: true}, "0.0.0.0", false},
		{"loopback development", OIDCOptions{AllowLoopbackHTTP: true}, "127.0.0.1", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := oidcAddressAllowed(tt.opts, net.ParseIP(tt.ip)); got != tt.want {
				t.Fatalf("oidcAddressAllowed(%s) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}
