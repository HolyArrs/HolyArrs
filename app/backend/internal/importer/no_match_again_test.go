package importer

import "testing"

// A download that matched no file can be imported again as it is: Import
// files by hand finds nothing in one that holds only archives.
func TestNoMatchCanBeImportedAgain(t *testing.T) {
	for _, why := range []string{noMovieWhy, noEpisodeWhy, noTrackWhy} {
		if !CanImportAgain(why) {
			t.Fatalf("%q should allow Import again", why)
		}
	}
	if CanImportAgain("a sample, not the full video") {
		t.Fatal("another reason should not")
	}
}
