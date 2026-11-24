package releasename

import "testing"

// "BDRemux" written as one word is a remux.
func TestBDRemuxOneWord(t *testing.T) {
	for _, name := range []string{
		"Captains Courageous 1937 BDRemux 1080p FLAC 2.0-UTOPIA",
		"Movie.2001.2160p.UHDRemux.HEVC-GRP",
		"Dune: Part Two (2024) UHD BDRemux 4K 2160p HDR10 Dolby Vision P7 H.265 2xUkr/Eng | Sub Ukr/Eng",
	} {
		r := Parse(name, Hints{})
		if r.Source.Value != "remux" {
			t.Errorf("%q: source %q, want remux", name, r.Source.Value)
		}
	}
	if r := Parse("Captains Courageous 1937 BDRemux 1080p FLAC 2.0-UTOPIA", Hints{}); r.Title.Value != "Captains Courageous" {
		t.Errorf("title %q", r.Title.Value)
	}
}
