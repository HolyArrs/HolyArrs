package plugins

import "testing"

// The pins are the main key and the offline backup key. A merge must never
// bring back an older or throwaway pin.
func TestPinnedMainKey(t *testing.T) {
	want := []string{
		"n5a8nBqYlumtATVUUeOUYSWgbWI/yi62H1xDhXx6jjY=", // main
		"dmodZ/Y7L1QCT4EqPgZCxDhe1ZpWz1xhCgnl6PJ/uNQ=", // backup
	}
	got := PinnedKeyStrings()
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("pinned keys %v, want %v", got, want)
	}
	if len(PinnedKeys()) != len(want) {
		t.Fatal("a pinned key is not a valid ed25519 public key")
	}
}
