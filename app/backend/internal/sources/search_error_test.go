package sources

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A Release that matched no Slot is refused in plain words that say what to
// do, not "say which Slot".
func TestSearchErrorNoSlotIsPlain(t *testing.T) {
	rec := httptest.NewRecorder()
	SearchError(rec, ErrNoSlot)
	body := rec.Body.String()
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rec.Code)
	}
	if strings.Contains(body, "Slot") || !strings.Contains(body, "pick it from the episode it is for") {
		t.Fatalf("body %s", body)
	}
}
