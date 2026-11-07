package source

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"testing"
)

func TestRefusesCrossHostRedirectAndKeepsKey(t *testing.T) {
	var leaked bool
	other := serve(t, func(w http.ResponseWriter, r *http.Request) { leaked = true })
	// Same server, different host name: 127.0.0.1 versus localhost.
	_, port, _ := net.SplitHostPort(other.Listener.Addr().String())
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
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

func TestAddressPolicyRefuses(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) { t.Error("request should not arrive") })
	c := New(Config{URL: srv.URL + "/api", Protocol: "torrent", AllowAddr: func(ip net.IP) bool { return !ip.IsLoopback() }})
	if _, err := c.Caps(context.Background()); err == nil {
		t.Fatal("policy should refuse loopback")
	}
}

func TestRefusesHTTPSToHTTPRedirect(t *testing.T) {
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
