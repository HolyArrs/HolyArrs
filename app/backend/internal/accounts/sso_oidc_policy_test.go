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
