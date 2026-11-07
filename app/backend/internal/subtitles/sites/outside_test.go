package sites

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestOutsideBreakerKeepsTransientStatusWhenSiteCallbackFails(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"busy"}`))
	}))
	t.Cleanup(srv.Close)

	c, err := newClient(Config{Name: "test", URL: srv.URL, HTTP: srv.Client(), GapMS: 1})
	if err != nil {
		t.Fatal(err)
	}
	c.status = func(*http.Response, []byte) error {
		return errors.New("site-specific failure")
	}
	for range 4 {
		if _, err := c.get(context.Background(), "/search", "", 1024, false); err == nil {
			t.Fatal("failing subtitle call returned no error")
		}
	}
	if got := hits.Load(); got != 3 {
		t.Fatalf("server hits = %d, want 3", got)
	}
}
