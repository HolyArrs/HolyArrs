package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/doctor"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A failed Test whose cause is not known looks at the address once, not
// once per answer field and again inside the database write.
// route: POST /api/v1/sources/{id}/test
func TestFailedTestProbesOnce(t *testing.T) {
	hang := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = c.Close()
		}
	}))
	t.Cleanup(hang.Close)
	var dials atomic.Int32
	old := probeDialOf
	probeDialOf = func(domain.Source) doctor.Dial {
		return func(ctx context.Context, network, addr string) (net.Conn, error) {
			dials.Add(1)
			var d net.Dialer
			return d.DialContext(ctx, network, addr)
		}
	}
	t.Cleanup(func() { probeDialOf = old })
	srv, _ := sourceServer(t)
	var x SourceView
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/sources", `{"name":"Hang","format":"torznab","url":"`+hang.URL+`/api","api_key":"k","allow_local_network":true}`, 201), &x)
	dials.Store(0)
	_ = do(t, "POST", fmt.Sprintf("%s/api/v1/sources/%d/test", srv.URL, x.ID), "", 200)
	if n := dials.Load(); n != 1 {
		t.Fatalf("probe dials = %d, want 1", n)
	}
}
