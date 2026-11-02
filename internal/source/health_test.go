package source

import (
	"testing"

	"github.com/CoderLuii/holyarrs/internal/domain"
)

func TestHealthReason(t *testing.T) {
	for kind, want := range map[string]string{KindRateLimited: "slowed", KindDown: "down", KindBroken: "down", KindBlocked: "blocked", KindLoginRefused: "key", "odd": "down"} {
		k := kind
		h := HealthOf(domain.Source{State: "paused", PauseReason: &k})
		if h.Reason != want {
			t.Errorf("%s: reason %q, want %q", kind, h.Reason, want)
		}
		if kind == KindLoginRefused && h.State != StateAuthFailed || kind != KindLoginRefused && h.State != StateUnreachable {
			t.Errorf("%s: state %q changed", kind, h.State)
		}
	}
	if h := HealthOf(domain.Source{State: "paused"}); h.Reason != "down" {
		t.Errorf("no reason: %q", h.Reason)
	}
	for _, st := range []string{"enabled", "disabled"} {
		if h := HealthOf(domain.Source{State: st}); h.Reason != "" {
			t.Errorf("%s: reason %q", st, h.Reason)
		}
	}
}
