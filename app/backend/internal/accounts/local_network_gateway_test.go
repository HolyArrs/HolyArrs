package accounts

import (
	"net"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// The settings answer says which address HolyArrs sees for this browser and
// which listed networks look like a container gateway.
func TestLocalNetSettingsShowAddressAndGateways(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	if resp, out := own.do(t, f, "PUT", "/api/v1/auth/settings", `{"local_networks":["172.17.0.1","192.168.1.0/24"]}`); resp.StatusCode != 200 {
		t.Fatalf("save: %d %s", resp.StatusCode, out)
	}
	_, out := own.do(t, f, "GET", "/api/v1/auth/settings", "")
	var v struct {
		Seen     string   `json:"seen_address"`
		Gateways []string `json:"gateway_networks"`
	}
	jsonOf(t, out, &v)
	if v.Seen != "127.0.0.1" {
		t.Errorf("seen_address %q in %s", v.Seen, out)
	}
	if !reflect.DeepEqual(v.Gateways, []string{"172.17.0.1/32"}) {
		t.Errorf("gateway_networks %v in %s", v.Gateways, out)
	}
	if strings.Contains(out, `"gateway_networks":null`) {
		t.Errorf("null list: %s", out)
	}
}

func TestGatewayNets(t *testing.T) {
	for _, c := range []struct {
		list []string
		self string
		want []string
	}{
		{[]string{"192.168.1.0/24"}, "192.168.1.20", nil},
		{[]string{"172.17.0.1/32", "10.88.0.0/16", "10.0.2.2/32", "192.168.65.0/24"}, "", []string{"172.17.0.1/32", "10.88.0.0/16", "10.0.2.2/32", "192.168.65.0/24"}},
		// A wide home range is fine, unless this browser itself arrives as a gateway.
		{[]string{"10.0.0.0/8"}, "10.0.0.5", nil},
		{[]string{"172.16.0.0/12"}, "172.18.0.1", []string{"172.16.0.0/12"}},
		{[]string{"192.168.1.0/24"}, "192.168.65.1", nil},
		{[]string{"junk", "fe80::/10"}, "::1", nil},
	} {
		got := gatewayNets(c.list, c.self)
		if len(got) == 0 && len(c.want) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("gatewayNets(%v, %q) = %v, want %v", c.list, c.self, got, c.want)
		}
	}
}

// Compose and user networks (172.18-31.x, Podman 10.89.x) have gateways too.
func TestGatewayNetsUserNetworks(t *testing.T) {
	for _, c := range []struct {
		list []string
		want []string
	}{
		{[]string{"172.18.0.1"}, []string{"172.18.0.1"}},
		{[]string{"172.31.0.0/16"}, []string{"172.31.0.0/16"}},
		{[]string{"10.89.3.1/32"}, []string{"10.89.3.1/32"}},
		{[]string{"10.89.0.0/24"}, []string{"10.89.0.0/24"}},
		// Home ranges that hold no x.y.0.1 gateway, or are wide, stay quiet.
		{[]string{"172.20.5.0/24", "172.16.0.0/12", "10.89.0.7"}, nil},
	} {
		got := gatewayNets(c.list, "")
		if len(got) == 0 && len(c.want) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("gatewayNets(%v) = %v, want %v", c.list, got, c.want)
		}
	}
}

// Behind a proxy that is not trusted the skip never applies, so the card
// shows no address and says the proxy is not trusted.
func TestSeenAddressUntrustedProxy(t *testing.T) {
	_, p, _ := net.ParseCIDR("10.10.0.0/16")
	a := &Auth{proxies: []*net.IPNet{p}}
	for _, c := range []struct {
		remote string
		hdr    map[string]string
		seen   string
		bad    bool
	}{
		{"192.168.1.5:1234", nil, "192.168.1.5", false},
		{"172.18.0.5:1234", map[string]string{"X-Forwarded-For": "192.168.1.5"}, "", true},
		{"172.18.0.5:1234", map[string]string{"X-Real-IP": "192.168.1.5"}, "", true},
		{"10.10.0.2:1234", map[string]string{"Forwarded": "for=192.168.1.5"}, "", true},
		{"10.10.0.2:1234", map[string]string{"X-Forwarded-For": "1.2.3.4, 192.168.1.5"}, "192.168.1.5", false},
	} {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = c.remote
		for k, v := range c.hdr {
			r.Header.Set(k, v)
		}
		seen, bad := a.seenAddress(r)
		if seen != c.seen || bad != c.bad {
			t.Errorf("%s %v: got %q %v, want %q %v", c.remote, c.hdr, seen, bad, c.seen, c.bad)
		}
	}
}
