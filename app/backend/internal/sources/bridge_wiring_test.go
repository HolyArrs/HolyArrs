package sources

import "testing"

// The routes use the BridgeService they were given, so they agree with the
// scheduler; without one they make a single service and keep it.
func TestHandlersUseInjectedBridgeService(t *testing.T) {
	s, v := openStore(t)
	svc := &BridgeService{Store: s, Vault: v}
	h := &Handlers{Store: s, Vault: v, Bridges: svc}
	if h.bridges() != svc {
		t.Fatal("injected service not used")
	}
	h2 := &Handlers{Store: s, Vault: v}
	a, b := h2.bridges(), h2.bridges()
	if a == nil || a != b {
		t.Fatal("fallback not stable")
	}
	if a.Store != s || a.Vault != v {
		t.Fatal("fallback lost the store or vault")
	}
}
