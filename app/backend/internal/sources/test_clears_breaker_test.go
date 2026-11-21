package sources

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// A Source that failed until its breaker opened answers a person's Test as soon as it is back.
// route: POST /api/v1/sources/{id}/test
func TestSourceTestClearsBreaker(t *testing.T) {
	var down atomic.Bool
	down.Store(true)
	idx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if down.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(sourceCaps))
	}))
	t.Cleanup(idx.Close)
	srv, _ := sourceServer(t)
	var x SourceView
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/sources", `{"name":"Flaky","format":"torznab","url":"`+idx.URL+`/api","api_key":"k","allow_local_network":true}`, 201), &x)
	test := fmt.Sprintf("%s/api/v1/sources/%d/test", srv.URL, x.ID)
	for range 8 {
		_ = do(t, "POST", test, "", 200)
	}
	down.Store(false)
	var res struct {
		OK   bool   `json:"ok"`
		Text string `json:"text"`
	}
	_ = json.Unmarshal(do(t, "POST", test, "", 200), &res)
	if !res.OK {
		t.Fatalf("test after recovery: %q", res.Text)
	}
}
