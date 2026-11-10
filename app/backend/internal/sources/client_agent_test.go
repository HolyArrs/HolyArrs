package sources

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSourceRequests_SendHolyArrsUserAgent(t *testing.T) {
	old := UserAgent
	UserAgent = "HolyArrs/9.9.9"
	t.Cleanup(func() { UserAgent = old })
	got := make(chan string, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Get("User-Agent")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><caps><limits max="50"/><searching><search available="yes" supportedParams="q"/></searching></caps>`))
	}))
	defer srv.Close()
	allowAll := func(net.IP) bool { return true }

	_, _ = New(Config{URL: srv.URL, AllowAddr: allowAll}).Caps(context.Background())
	if ua := <-got; ua != "HolyArrs/9.9.9" {
		t.Fatalf("Source client sent %q", ua)
	}

	hc := &http.Client{Transport: Transport(Config{AllowAddr: allowAll})}
	resp, err := hc.Get(srv.URL + "/x.nzb")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if ua := <-got; ua != "HolyArrs/9.9.9" {
		t.Fatalf("release fetch sent %q", ua)
	}

	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	req.Header.Set("User-Agent", "Caller/1")
	resp, err = hc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if ua := <-got; ua != "Caller/1" {
		t.Fatalf("caller's User-Agent replaced: %q", ua)
	}
}
