package migrate

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// A bad move id answers that the move is missing, not a snapshot.
func TestBadMoveIDSaysMoveNotFound(t *testing.T) {
	for _, id := range []string{"abc", "0", "-3"} {
		r := httptest.NewRequest("GET", "/api/v1/migrate/moves/"+id, nil)
		r.SetPathValue("id", id)
		w := httptest.NewRecorder()
		if _, ok := moveID(w, r); ok {
			t.Fatalf("id %q accepted", id)
		}
		if w.Code != 404 || !strings.Contains(w.Body.String(), ErrNoMove.Error()) {
			t.Fatalf("id %q: got %d %s", id, w.Code, w.Body.String())
		}
	}
}
