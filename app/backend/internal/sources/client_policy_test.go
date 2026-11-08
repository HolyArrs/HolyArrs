package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestPolicy_DefaultRefusesLocalNetwork checks that the local network is refused unless the Source allows it.
func TestPolicy_DefaultRefusesLocalNetwork(t *testing.T) {
	srv := okServer(t)
	// The check runs on the address being dialled, after DNS, so a name that points at a local address is refused too and rebinding cannot slip by.
	for _, u := range []string{srv.URL, strings.Replace(srv.URL, "127.0.0.1", "localhost", 1)} {
		_, err := New(Config{URL: u + "/api", Protocol: "torrent"}).Caps(context.Background())
		if err == nil || !strings.Contains(err.Error(), "network policy") {
			t.Fatalf("%s: %v", u, err)
		}
	}
}

// TestPolicy_AllowLocalNetworkPerSource checks that the local network setting works per Source.
func TestPolicy_AllowLocalNetworkPerSource(t *testing.T) {
	srv := okServer(t)
	if _, err := New(Config{URL: srv.URL + "/api", Protocol: "torrent", AllowLocal: true}).Caps(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// TestPolicy_AllowLocalStillRefusesMetadata checks that allowing the local network still refuses cloud metadata addresses.
func TestPolicy_AllowLocalStillRefusesMetadata(t *testing.T) {
	for _, host := range []string{"169.254.169.254", "100.100.100.200", "[fd00:ec2::254]"} {
		_, err := New(Config{URL: "http://" + host + "/api", Protocol: "torrent", AllowLocal: true}).Caps(context.Background())
		if err == nil || !strings.Contains(err.Error(), "network policy") {
			t.Fatalf("metadata address %s: %v", host, err)
		}
	}
}

// TestPolicy_IgnoresProxySettings checks that the address policy applies even when proxy environment settings are present.
func TestPolicy_IgnoresProxySettings(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://8.8.8.8:3128")
	c := New(Config{URL: "http://127.0.0.1:1/api", Protocol: "torrent"})
	tr := c.http.Transport.(*http.Transport)
	if tr.Proxy != nil {
		t.Fatal("a proxy would hide the real address from the policy")
	}
}

// okServer starts a local server that answers every request with a valid empty feed.
func okServer(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<caps><searching><search available="yes" supportedParams="q"/></searching></caps>`))
	}))
	t.Cleanup(srv.Close)
	return srv
}
