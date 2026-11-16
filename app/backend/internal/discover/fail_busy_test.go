package discover

import (
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// A busy answer while the server asks to wait names the reason and asks
// the page to come back only when the wait ends.
func TestFailBusyNamesReasonAndWait(t *testing.T) {
	metadata.ResetRefusal()
	t.Cleanup(metadata.ResetRefusal)
	metadata.SetRefusal(metadata.Refusal{Code: "registration_limit", Until: time.Now().Add(10 * time.Minute)})
	w := httptest.NewRecorder()
	Fail(w, metadata.ErrRateLimited)
	if w.Code != 503 {
		t.Fatalf("status %d", w.Code)
	}
	ra, _ := strconv.Atoi(w.Header().Get("Retry-After"))
	if ra < 590 || ra > 600 {
		t.Fatalf("Retry-After %q", w.Header().Get("Retry-After"))
	}
	if !strings.Contains(w.Body.String(), "/config") {
		t.Fatalf("body names no reason: %s", w.Body)
	}
}
