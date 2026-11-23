package library

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// In a flat folder a subtitle belongs to a movie only by its exact base
// name plus a language suffix. A subtitle named after another movie whose
// name starts with this one's stays with that movie.
func TestFlatSubtitleNeedsExactBaseName(t *testing.T) {
	root := t.TempDir()
	for _, n := range []string{"Up.mkv", "Up.en.srt", "Up.en.forced.srt", "Up.De.mkv", "Up.De.srt", "Up.De.fr.srt", "Updog.en.srt", "Up.nl.mp4", "Up.nl.en.srt"} {
		if err := os.WriteFile(filepath.Join(root, n), []byte("1\n00:00:01,000 --> 00:00:02,000\nhi\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct {
		video string
		want  []string
	}{
		{"Up.mkv", []string{"Up.en.forced.srt", "Up.en.srt"}},
		{"Up.De.mkv", []string{"Up.De.fr.srt"}},
		{"Up.nl.mp4", []string{"Up.nl.en.srt"}},
	} {
		var got []string
		for _, g := range besideSubtitles(root, c.video) {
			got = append(got, g...)
		}
		slices.Sort(got)
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: got %v want %v", c.video, got, c.want)
		}
	}
}
