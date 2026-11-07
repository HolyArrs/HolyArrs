package metadata

import (
	"errors"
	"testing"
)

func TestFirstCreditID(t *testing.T) {
	id, err := firstCreditID([]byte(`{"id":"x","artist-credit":[{"artist":{"id":"A0000000-0000-0000-0000-0000000000AA"}},{"artist":{"id":"b0000000-0000-0000-0000-0000000000bb"}}]}`))
	if err != nil || id != "a0000000-0000-0000-0000-0000000000aa" {
		t.Fatalf("got %q %v", id, err)
	}
	for _, b := range []string{`{"id":"x"}`, `{"artist-credit":[{"artist":{"id":"nope"}}]}`} {
		if _, err := firstCreditID([]byte(b)); !errors.Is(err, ErrArtistNotFound) {
			t.Fatalf("%s: %v", b, err)
		}
	}
}
