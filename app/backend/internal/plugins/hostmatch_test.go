package plugins

import (
	"net/netip"
	"testing"
)

func TestMatchHost(t *testing.T) {
	pats := []string{"*.tailscale.com", "controlplane.tailscale.com", "api.example.org"}
	for name, want := range map[string]bool{
		"tailscale.com":             false,
		"derp1.tailscale.com":       true,
		"a.b.tailscale.com":         true,
		"evil-tailscale.com":        false,
		"tailscale.com.evil.com":    false,
		"eviltailscale.com":         false,
		"DERP1.Tailscale.COM.":      true,
		"api.example.org":           true,
		"x.api.example.org":         false,
		"xn--bcher-kva.example.org": false,
		"127.0.0.1":                 false,
		"":                          false,
		"bücher.example.org":        false,
	} {
		if got := matchHost(pats, name); got != want {
			t.Errorf("matchHost(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestIsIPLiteral(t *testing.T) {
	for _, s := range []string{"1.2.3.4", "1.2.3.4.", "0x7f.1", "2130706433", "017700000001", "[::1]", "::1", "fe80::1%eth0", "1.2.3", "a.b.123", "a.0x10"} {
		if !isIPLiteral(s) {
			t.Errorf("%q is not seen as an address", s)
		}
	}
	for _, s := range []string{"example.com", "1password.com", "a1.b2.c3d", "derp1.tailscale.com"} {
		if isIPLiteral(s) {
			t.Errorf("%q is seen as an address", s)
		}
	}
}

func TestValidHostPattern(t *testing.T) {
	for p, want := range map[string]bool{
		"controlplane.tailscale.com": true,
		"*.tailscale.com":            true,
		"*.com":                      false,
		"a.*.com":                    false,
		"**.x.com":                   false,
		"Example.com":                false,
		"example.com:443":            false,
		"1.2.3.4":                    false,
		"*.1.2.3":                    false,
		"https://x.com":              false,
		"localhost":                  true,
	} {
		if got := validHostPattern(p); got != want {
			t.Errorf("validHostPattern(%q) = %v, want %v", p, got, want)
		}
	}
}

func TestSafeAddr(t *testing.T) {
	own := []netip.Addr{netip.MustParseAddr("203.0.113.9")}
	for s, want := range map[string][2]bool{ // [listed host, owner host]
		"93.184.216.34":          {true, true},
		"10.0.0.1":               {false, true},
		"192.168.1.5":            {false, true},
		"172.16.0.1":             {false, true},
		"100.64.0.1":             {false, true},
		"127.0.0.1":              {false, false},
		"0.0.0.0":                {false, false},
		"169.254.169.254":        {false, false},
		"224.0.0.1":              {false, false},
		"::1":                    {false, false},
		"fe80::1":                {false, false},
		"fd00::1":                {false, true},
		"::ffff:127.0.0.1":       {false, false},
		"::ffff:10.0.0.1":        {false, true},
		"::ffff:93.184.216.34":   {true, true},
		"64:ff9b::7f00:1":        {false, false},
		"2002:7f00:1::":          {false, false},
		"2606:2800:220:1::1":     {true, true},
		"203.0.113.9":            {false, false},
		"::ffff:203.0.113.9":     {false, false},
		"64:ff9b::5db8:d822":     {true, true},
		"2002:a00:1::1":          {false, true},
		"ff02::1":                {false, false},
		"198.18.0.1":             {false, true},
		"2001:db8::1":            {false, true},
		"100::1":                 {false, true},
		"192.0.0.8":              {false, true},
		"240.0.0.1":              {false, false},
		"255.255.255.255":        {false, false},
		"fe80::1%eth0":           {false, false},
		"::":                     {false, false},
		"64:ff9b:1::a00:1":       {false, true},
		"2001:4860:4860::8888":   {true, true},
		"8.8.8.8":                {true, true},
		"::ffff:169.254.169.254": {false, false},
	} {
		a := netip.MustParseAddr(s)
		if got := safeAddr(a, false, own); got != want[0] {
			t.Errorf("safeAddr(%s, listed) = %v, want %v", s, got, want[0])
		}
		if got := safeAddr(a, true, own); got != want[1] {
			t.Errorf("safeAddr(%s, owner) = %v, want %v", s, got, want[1])
		}
	}
}

func TestParseOwnerHost(t *testing.T) {
	for s, want := range map[string]bool{
		"headscale.home.arpa":      true,
		"headscale.home.arpa:8080": true,
		"hs.example.com:0":         false,
		"hs.example.com:70000":     false,
		"192.168.1.5":              false,
		"192.168.1.5:8080":         false,
		"[::1]:80":                 false,
		"localhost":                false,
		"hs.example.com:":          false,
	} {
		if _, _, ok := ParseOwnerHost(s); ok != want {
			t.Errorf("ParseOwnerHost(%q) ok = %v, want %v", s, ok, want)
		}
	}
}

func TestSafeAddrRefusesOldIPv6Forms(t *testing.T) {
	for _, s := range []string{"::a00:1", "::7f00:1", "::ffff:0:a00:1", "fec0::1", "2001:0:4136:e378:8000:63bf:f5ff:fffe", "192.88.99.1"} {
		ip := netip.MustParseAddr(s)
		if safeAddr(ip, false, nil) || safeAddr(ip, true, nil) {
			t.Errorf("%s was accepted", s)
		}
	}
}
