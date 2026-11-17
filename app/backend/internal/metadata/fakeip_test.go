package metadata

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

func TestUnreachedNamesFakeIP(t *testing.T) {
	err := unreached(fmt.Errorf("get: %w", &outbound.FakeIPError{Host: "api.holyarrs.com"}))
	if !errors.Is(err, ErrServiceUnavailable) {
		t.Fatal("a fake address still counts as the server being unreachable")
	}
	if FakeIPHost(err) != "api.holyarrs.com" || !strings.Contains(err.Error(), "Clash or Surge") {
		t.Fatalf("err = %v", err)
	}
	if FakeIPHost(ErrServiceUnavailable) != "" {
		t.Fatal("plain down is not a fake address")
	}
}

func TestHealthHandlerFakeIP(t *testing.T) {
	m := http.NewServeMux()
	(&Handlers{Up: func(context.Context) bool { return false }, FakeIP: func() string { return "api.holyarrs.com" }}).Routes(m)
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/metadata/health", nil))
	b := rec.Body.String()
	if !strings.Contains(b, `"fakeIP":"api.holyarrs.com"`) || !strings.Contains(b, "198.18.x.x") {
		t.Fatalf("body: %s", b)
	}
}
