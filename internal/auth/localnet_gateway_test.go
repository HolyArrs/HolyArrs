package auth

import (
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
