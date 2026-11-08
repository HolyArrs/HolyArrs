package accounts

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// An open stream follows its session through the hourly cookie change, and
// stops once the person signs out or must turn on two-factor first.
func TestStillFollowsTheSessionFamily(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	ck := own.c.Jar.Cookies(mustURL(f.srv.URL))[0]
	stream := func(family bool) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/live", nil)
		r.AddCookie(&http.Cookie{Name: CookieName, Value: ck.Value})
		r = r.WithContext(webserver.ContextWith(context.Background(), webserver.Caller{UserID: 1, Via: "session", Role: webserver.RoleOwner}))
		if family {
			r = f.a.WithSessionFamily(r)
		}
		return r
	}
	open, bare := stream(true), stream(false)
	if _, ok := f.a.Still(open); !ok {
		t.Fatal("a fresh stream is refused")
	}
	// The browser goes on with a new cookie; the old one stops a minute later.
	f.clock.add(RotateEvery + time.Minute)
	if resp, _ := own.do(t, f, http.MethodGet, "/api/v1/auth/me", ""); resp.StatusCode != http.StatusOK {
		t.Fatal(resp.StatusCode)
	}
	f.clock.add(rotateGrace + time.Minute)
	if _, ok := f.a.Still(bare); ok {
		t.Fatal("the replaced cookie still works on its own (the check is not testing rotation)")
	}
	who, ok := f.a.Still(open)
	if !ok || who.UserID == 0 || who.Role != webserver.RoleOwner {
		t.Fatalf("stream lost its sign-in at the hourly cookie change: %+v %v", who, ok)
	}
	if resp, _ := own.do(t, f, http.MethodPost, "/api/v1/auth/logout", ""); resp.StatusCode != http.StatusNoContent {
		t.Fatal(resp.StatusCode)
	}
	if _, ok := f.a.Still(open); ok {
		t.Fatal("stream kept working after sign-out")
	}
}
