package sources

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Each person gets bridgeTries tries a minute; the window slides.
func TestBridgeFindRateLimited(t *testing.T) {
	var l bridgeLimiter
	now := time.Now()
	for i := 0; i < bridgeTries; i++ {
		if !l.allow(1, now) {
			t.Fatalf("try %d refused", i)
		}
	}
	if l.allow(1, now) {
		t.Fatal("over the cap")
	}
	if !l.allow(2, now) {
		t.Fatal("another person shares the bucket")
	}
	if !l.allow(1, now.Add(61*time.Second)) {
		t.Fatal("still refused after a minute")
	}

	// Through the handler: a 429 with Retry-After, per person.
	h := &Handlers{}
	try := func(user int64) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/api/v1/bridges/find", nil)
		r = r.WithContext(webserver.ContextWith(r.Context(), webserver.Caller{UserID: user}))
		rec := httptest.NewRecorder()
		if h.bridgeAllowed(rec, r) {
			rec.WriteHeader(http.StatusOK)
		}
		return rec
	}
	for i := 0; i < bridgeTries; i++ {
		if rec := try(1); rec.Code != http.StatusOK {
			t.Fatalf("try %d: %d", i, rec.Code)
		}
	}
	rec := try(1)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "60" {
		t.Fatalf("over the cap: %d %v", rec.Code, rec.Header())
	}
	if rec := try(2); rec.Code != http.StatusOK {
		t.Fatalf("another person: %d", rec.Code)
	}
}
