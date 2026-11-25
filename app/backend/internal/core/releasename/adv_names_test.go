package releasename

import "testing"

// Invented names where a quality word is also a title word, and DTS:X
// written with a dot.
func TestAdvTitleWordTags(t *testing.T) {
	cases := []struct {
		name, title   string
		year          int
		source, audio string
	}{
		{"The.Cam.2021.1080p.WEB-DL.x264-GRP", "The Cam", 2021, "web_dl", ""},
		{"A.Complete.Unknown.2024.1080p.WEB-DL-GRP", "A Complete Unknown", 2024, "web_dl", ""},
		{"Movie.2020.CAM.XviD-GRP", "Movie", 2020, "cam", ""},
		{"Movie.CAM.XviD-GRP", "Movie", 0, "cam", ""},
		{"Movie.2020.COMPLETE.BLURAY-GRP", "Movie", 2020, "disc_image", ""},
		{"Movie.Extended.Cut.2020.1080p-GRP", "Movie", 0, "", ""},
		{"Movie.2020.1080p.BluRay.DTS.X.7.1-GRP", "Movie", 2020, "bluray", "dts_x"},
	}
	for _, c := range cases {
		r := Parse(c.name, Hints{})
		if r.Title.Value != c.title || r.Year.Value != c.year || r.Source.Value != c.source || c.audio != "" && r.Audio.Format != c.audio {
			t.Errorf("%s: got %q %d %q %q", c.name, r.Title.Value, r.Year.Value, r.Source.Value, r.Audio.Format)
		}
	}
}
