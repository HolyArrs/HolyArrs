package accounts

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

// The OIDC client dials through the shared address policy with the options of
// its provider, so a private or metadata address is refused when the
// options say so, however the name resolves.
func TestOIDCClientDialsThroughThePolicy(t *testing.T) {
	t.Parallel()
	issuer, _ := url.Parse("https://id.example.com")
	tests := []struct {
		name string
		opts OIDCOptions
		addr string
	}{
		{"private by default", OIDCOptions{}, "10.0.0.4:443"},
		{"loopback by default", OIDCOptions{}, "127.0.0.1:443"},
		{"metadata when local is allowed", OIDCOptions{AllowLocal: true}, "169.254.169.254:80"},
		{"multicast when local is allowed", OIDCOptions{AllowLocal: true}, "224.0.0.1:443"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newOIDCHTTPClient(tt.opts, issuer)
			base := client.Transport.(oidcLimitTransport).base.(*http.Transport)
			conn, err := base.DialContext(context.Background(), "tcp", tt.addr)
			if err == nil {
				conn.Close()
				t.Fatalf("dialing %s was allowed", tt.addr)
			}
			if !errors.Is(err, outbound.ErrAddressRefused) {
				t.Fatalf("dialing %s failed for another reason: %v", tt.addr, err)
			}
		})
	}
}

// Google hands out tokens and keys from googleapis.com, so its discovery
// document passes; other issuers still keep every endpoint on their own host.
func TestOIDCEndpointsAllowGoogleSiblingHosts(t *testing.T) {
	t.Parallel()
	google, _ := url.Parse("https://accounts.google.com")
	other, _ := url.Parse("https://id.example.com")
	plain, _ := url.Parse("http://accounts.google.com")
	tests := []struct {
		issuer *url.URL
		raw    string
		want   bool
	}{
		{google, "https://accounts.google.com/o/oauth2/v2/auth", true},
		{google, "https://oauth2.googleapis.com/token", true},
		{google, "https://www.googleapis.com/oauth2/v3/certs", true},
		{google, "https://openidconnect.googleapis.com/v1/userinfo", true},
		{google, "https://evil.example.com/token", false},
		{google, "https://oauth2.googleapis.com:8443/token", false},
		{google, "http://oauth2.googleapis.com/token", false},
		{plain, "http://oauth2.googleapis.com/token", false},
		{other, "https://oauth2.googleapis.com/token", false},
		{other, "https://id.example.com/token", true},
	}
	for _, tt := range tests {
		if got := oidcEndpointAllowed(tt.issuer, tt.raw); got != tt.want {
			t.Errorf("issuer %s endpoint %s: got %v, want %v", tt.issuer, tt.raw, got, tt.want)
		}
	}
}
