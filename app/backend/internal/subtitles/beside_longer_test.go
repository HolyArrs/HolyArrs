package subtitles

import (
	"os"
	"path/filepath"
	"testing"
)

// In a flat folder Up.De.srt belongs to Up.De.mkv, not to Up.mkv, even
// though "de" reads as a language.
func TestBesideSkipsSubtitleOfLongerVideo(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"Up.mkv", "Up.en.srt", "Up.De.mkv", "Up.De.srt", "Up.De.en.srt"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	longer := longerVideos("Up", ents)
	for _, c := range []struct {
		name string
		want bool
	}{{"Up.en.srt", true}, {"Up.De.srt", false}, {"Up.De.en.srt", false}, {"up.de.SRT", false}} {
		_, ok := besideName("Up", c.name)
		if got := ok && !ownedByLonger(longer, c.name); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}
