package discover

import "testing"

// No artwork is an empty list; artwork that is not a cached poster is kept as it is.
func TestArtworkLinksKeepsOtherArtwork(t *testing.T) {
	t.Parallel()
	if got := ArtworkLinks(nil); got == nil || len(got) != 0 {
		t.Fatalf("no artwork must be an empty list, got %#v", got)
	}
	if got := ArtworkLinks([]string{"https://image.tmdb.org/t/p/w342/x.jpg", "poster:"}); got[0] != "https://image.tmdb.org/t/p/w342/x.jpg" || got[1] != "poster:" {
		t.Fatalf("other artwork must be kept as it is: %#v", got)
	}
}
