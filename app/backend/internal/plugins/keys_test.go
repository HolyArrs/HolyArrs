package plugins

import "testing"

// 1.3.0 pins one key: the plugins repo's signing key. A merge must never
// bring back an older or throwaway pin.
func TestPinnedMainKey(t *testing.T) {
	const main = "r66aC5kfjz7wBDfiPb94o0RlpSm4F2fwfr8Kk/KNYGw="
	got := PinnedKeyStrings()
	if len(got) != 1 || got[0] != main {
		t.Fatalf("pinned keys %v, want only %s", got, main)
	}
	if len(PinnedKeys()) != 1 {
		t.Fatal("the pinned main key is not a valid ed25519 public key")
	}
}
