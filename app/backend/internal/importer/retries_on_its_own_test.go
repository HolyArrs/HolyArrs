package importer

import "testing"

func TestRetriesOnItsOwn(t *testing.T) {
	cases := []struct {
		name   string
		reason string
		want   bool
	}{
		{"file not where the Downloader says", notThereWhy, true},
		{"parent exists but movie has not arrived", fileTakenWhy, true},
		{"listed file has not reached the local folder yet", NotPresentYetWhy, true},
		{"episode has not arrived", "the file for S02E08 could not be placed: " + notInFolderWhy, true},
		{"track has not arrived", "the file for Track 02 could not be placed: " + notInFolderWhy, true},
		{"file still copying", CopyingWhy, true},
		{"not another app's completed import", FilesGoneReason, false},
		{"unrelated text with missing suffix", "bad release: " + notInFolderWhy, false},
		{"a read permission waits for a person", noReadWhy, false},
		{"import gave up on a passing fault", gaveUpWhy, true},
		{"folder read took too long", SlowReadWhy, true},
		{"no file matched the movie: by hand", noMovieWhy, false},
		{"no file matched an episode: by hand", noEpisodeWhy, false},
		{"no file matched a track: by hand", noTrackWhy, false},
		{"Library not writable waits for a person", noWriteWhy, false},
		{"save folder not set waits for a person", "HolyArrs can't read this download's files: HOLYARRS_SAVE_PATH is not set", false},
		{"name taken in the Library waits for a person", targetTakenWhy, false},
		{"empty", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := RetriesOnItsOwn(c.reason); got != c.want {
				t.Fatalf("RetriesOnItsOwn(%q) = %v, want %v", c.reason, got, c.want)
			}
		})
	}
}
