package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthHandler(t *testing.T) {
	get := func(h *Handlers) string {
		m := http.NewServeMux()
		h.Routes(m)
		rec := httptest.NewRecorder()
		m.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/metadata/health", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
		return rec.Body.String()
	}
	if b := get(&Handlers{}); !strings.Contains(b, `"up":true`) || strings.Contains(b, `"message"`) {
		t.Fatalf("nil Up: %s", b)
	}
	if b := get(&Handlers{Up: func(context.Context) bool { return true }}); !strings.Contains(b, `"up":true`) || strings.Contains(b, `"message"`) {
		t.Fatalf("up: %s", b)
	}
	b := get(&Handlers{Up: func(context.Context) bool { return false }})
	if !strings.Contains(b, `"up":false`) || !strings.Contains(b, `"message"`) || !strings.Contains(b, "Metadata server is down") {
		t.Fatalf("down: %s", b)
	}
}
