package source

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
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

func okServer(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<caps><searching><search available="yes" supportedParams="q"/></searching></caps>`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDefaultRefusesLocalNetwork(t *testing.T) {
	srv := okServer(t)
	// By IP, and by a name that resolves to a local address: the check runs
	// on the address being dialled, after DNS, so rebinding cannot slip by.
	for _, u := range []string{srv.URL, strings.Replace(srv.URL, "127.0.0.1", "localhost", 1)} {
		_, err := New(Config{URL: u + "/api", Protocol: "torrent"}).Caps(context.Background())
		if err == nil || !strings.Contains(err.Error(), "network policy") {
			t.Fatalf("%s: %v", u, err)
		}
	}
}

func TestAllowLocalNetworkPerSource(t *testing.T) {
	srv := okServer(t)
	if _, err := New(Config{URL: srv.URL + "/api", Protocol: "torrent", AllowLocal: true}).Caps(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestAllowLocalStillRefusesMetadata(t *testing.T) {
	for _, host := range []string{"169.254.169.254", "100.100.100.200", "[fd00:ec2::254]"} {
		_, err := New(Config{URL: "http://" + host + "/api", Protocol: "torrent", AllowLocal: true}).Caps(context.Background())
		if err == nil || !strings.Contains(err.Error(), "network policy") {
			t.Fatalf("metadata address %s: %v", host, err)
		}
	}
}

func TestPolicyIgnoresProxySettings(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://8.8.8.8:3128")
	c := New(Config{URL: "http://127.0.0.1:1/api", Protocol: "torrent"})
	tr := c.http.Transport.(*http.Transport)
	if tr.Proxy != nil {
		t.Fatal("a proxy would hide the real address from the policy")
	}
}
