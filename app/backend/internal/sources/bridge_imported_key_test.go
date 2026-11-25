package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"net/url"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

func TestImportedKeyOnlySameProwlarrAndOwner(t *testing.T) {
	u := func(s string) *url.URL { x, _ := url.Parse(s); return x }
	for _, tc := range []struct {
		stored, want string
		ok           bool
	}{
		{"https://p.example:9696/5/api", "https://p.example:9696", true},
		{"https://p.example:9696/5/api", "http://p.example:9696", false},
		{"http://p.example:9696/5/api", "http://p.example:9697", false},
		{"http://p.example:9696/5/api", "http://q.example:9696", false},
		{"http://p.example:9696/api", "http://p.example:9696", false},
		{"http://P.Example:9696/5/api", "http://p.example:9696", true},
		{"http://p.example:9696/5/api", "HTTP://P.EXAMPLE:9696", true},
	} {
		if got := sameProwlarr(u(tc.stored), u(tc.want)); got != tc.ok {
			t.Errorf("%s vs %s: got %v", tc.stored, tc.want, got)
		}
	}
	for _, tc := range []struct {
		c  webserver.Caller
		ok bool
	}{
		{webserver.Caller{Role: webserver.RoleOwner, Via: "session"}, true},
		{webserver.Caller{Role: webserver.RoleOwner, Via: "api_key"}, false},
		{webserver.Caller{Role: webserver.RoleAdmin, Via: "session"}, false},
	} {
		if got := ownerInBrowser(tc.c); got != tc.ok {
			t.Errorf("%+v: got %v", tc.c, got)
		}
	}
}

// The key goes only in the X-Api-Key header, and the words shown for a
// failure never carry it, even when Prowlarr echoes it back.
func TestProwlarrKeyOnlyInHeaderNeverInError(t *testing.T) {
	const key = "k3y-SECRET-0123456789"
	var urlSeen, bodySeen string
	var headerSeen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		urlSeen = r.URL.String()
		headerSeen = r.Header.Get("X-Api-Key")
		b := make([]byte, 64)
		n, _ := r.Body.Read(b)
		bodySeen = string(b[:n])
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`[{"propertyName":"x","errorMessage":"bad key ` + key + `"}]`))
	}))
	defer srv.Close()
	_, err := NewBridgeProwlarr(srv.URL, key).Status(context.Background())
	if err == nil {
		t.Fatal("want an error")
	}
	if headerSeen != key {
		t.Errorf("header: %q", headerSeen)
	}
	if strings.Contains(urlSeen, key) || strings.Contains(bodySeen, key) {
		t.Errorf("key outside the header: %q %q", urlSeen, bodySeen)
	}
	if strings.Contains(err.Error(), key) || strings.Contains(BridgeWords(err), key) {
		t.Errorf("key in the error: %v", err)
	}
	// Status codes and refused keys carry no key either.
	for _, code := range []int{401, 404, 500} {
		s2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(code) }))
		_, err := NewBridgeProwlarr(s2.URL, key).Status(context.Background())
		s2.Close()
		if err == nil || strings.Contains(BridgeWords(err), key) {
			t.Errorf("status %d: %v", code, err)
		}
	}
}
