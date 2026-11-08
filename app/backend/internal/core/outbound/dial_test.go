package outbound

import (
	"net"
	"testing"
)

func TestPublicOnly(t *testing.T) {
	for addr, ok := range map[string]bool{
		"127.0.0.1": false, "10.1.2.3": false, "172.16.0.9": false, "192.168.1.1": false, "169.254.169.254": false,
		"100.64.0.1": false, "100.127.255.254": false, "0.0.0.0": false, "224.0.0.1": false, "255.255.255.255": false,
		"::1": false, "fe80::1": false, "fc00::1": false, "fd12::1": false, "::ffff:10.0.0.1": false, "::": false,
		"0.1.2.3": false, "198.18.0.1": false, "198.19.255.1": false, "240.0.0.1": false, "64:ff9b::a00:1": false,
		"64:ff9b:1::1": false, "2002:a00:1::1": false, "198.20.0.1": true, "fec0::1": false, "::a00:1": false,
		"8.8.8.8": true, "1.1.1.1": true, "100.128.0.1": true, "2001:4860:4860::8888": true,
	} {
		if got := PublicOnly(net.ParseIP(addr)); got != ok {
			t.Errorf("PublicOnly(%s) = %v, want %v", addr, got, ok)
		}
	}
}

func TestNotMetadataOddRanges(t *testing.T) {
	for addr, want := range map[string]bool{
		"0.0.0.0": false, "::": false, "224.0.0.1": false, "255.255.255.255": false,
		"64:ff9b::a9fe:a9fe": false, "192.0.0.192": false, "2002:a9fe:a9fe::": false,
		"192.168.1.2": true, "127.0.0.1": true, "10.0.0.5": true,
	} {
		if got := NotMetadata(net.ParseIP(addr)); got != want {
			t.Errorf("NotMetadata(%s) = %v, want %v", addr, got, want)
		}
	}
}
