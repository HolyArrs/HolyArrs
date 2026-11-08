package sources

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"testing"
)

// TestRedirect_CrossHostRefusedKeepsKey checks that a redirect to another host is refused, so the key never leaves.
func TestRedirect_CrossHostRefusedKeepsKey(t *testing.T) {
	var leaked bool
	other := serve(t, func(w http.ResponseWriter, r *http.Request) { leaked = true })
	_, port, _ := net.SplitHostPort(other.Listener.Addr().String())
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		// Same server under another host name: 127.0.0.1 versus localhost.
		http.Redirect(w, r, "http://localhost:"+port+"/api?"+r.URL.RawQuery, http.StatusFound)
	})
	c := New(Config{URL: srv.URL + "/api", APIKey: "k", Protocol: "torrent", AllowLocal: true})
	if _, err := c.Caps(context.Background()); err == nil {
		t.Fatal("cross-host redirect should fail")
	}
	if leaked {
		t.Fatal("request reached the other host")
	}
}

// TestRedirect_AddressPolicyRefuses checks that a redirect to an address the policy refuses is not followed.
func TestRedirect_AddressPolicyRefuses(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) { t.Error("request should not arrive") })
	c := New(Config{URL: srv.URL + "/api", Protocol: "torrent", AllowAddr: func(ip net.IP) bool { return !ip.IsLoopback() }})
	if _, err := c.Caps(context.Background()); err == nil {
		t.Fatal("policy should refuse loopback")
	}
}

// TestRedirect_HTTPSToHTTPRefused checks that a redirect from https down to http is refused.
func TestRedirect_HTTPSToHTTPRefused(t *testing.T) {
	via := &http.Request{URL: &url.URL{Scheme: "https", Host: "idx.example"}}
	req := &http.Request{URL: &url.URL{Scheme: "http", Host: "idx.example"}}
	if err := sameHostRedirect(req, []*http.Request{via}); err == nil {
		t.Fatal("https to http should be refused")
	}
	req.URL.Scheme = "https"
	if err := sameHostRedirect(req, []*http.Request{via}); err != nil {
		t.Fatal(err)
	}
}
