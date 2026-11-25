package accounts

import "testing"

// TestHomeAddress: plain http is allowed only for home-network addresses.
func TestHomeAddress(t *testing.T) {
	for addr, want := range map[string]bool{
		"http://10.10.25.50:8096":     true,
		"http://192.168.1.20:8096":    true,
		"http://127.0.0.1:8096":       true,
		"http://nas:8096":             true,
		"http://tower.local:8096":     true,
		"http://media.home.arpa:8096": true,
		"http://8.8.8.8:8096":         false,
		"http://jellyfin.example.com": false,
		"http://203.0.113.9:8096":     false,
	} {
		if got := homeAddress(addr); got != want {
			t.Errorf("homeAddress(%q) = %v, want %v", addr, got, want)
		}
	}
}

// TestServerAddressPicksScheme: a typed address with no scheme gets http and
// port 8096 at home, https outside.
func TestServerAddressPicksScheme(t *testing.T) {
	for in, want := range map[string]string{
		"10.10.25.50":             "http://10.10.25.50:8096",
		"10.10.25.50:8097":        "http://10.10.25.50:8097",
		"jellyfin.example.com":    "https://jellyfin.example.com",
		"http://10.10.25.50:8096": "http://10.10.25.50:8096",
	} {
		if got, ok := serverAddress(in); !ok || got != want {
			t.Errorf("serverAddress(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
}
