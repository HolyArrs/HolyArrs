package releasename

import (
	"slices"
	"testing"
)

func TestWithoutTitleWords(t *testing.T) {
	cases := []struct {
		name   string
		show   bool
		titles []string
		want   []string
	}{
		{"Show.Name.S01E01.The.German.Job.1080p.WEB.h264-GRP", true, []string{"Show Name", "The German Job"}, nil},
		{"Show.Name.S01E01.The.German.Job.GERMAN.1080p.WEB.h264-GRP", true, []string{"Show Name", "The German Job"}, []string{"de"}},
		{"Show.Name.S02E03.French.Kiss.1080p.WEB.h264-GRP", true, []string{"Show Name", "French Kiss"}, nil},
		{"Show.Name.S02E03.French.Kiss.FRENCH.720p.HDTV.x264-GRP", true, []string{"Show Name", "French Kiss"}, []string{"fr"}},
		{"Show.Name.S01E05.FRENCH.1080p.WEB.h264-GRP", true, []string{"Show Name", "French"}, []string{"fr"}},
		{"Show.Name.S01E01.The.German.Job.1080p.WEB.h264-GRP", true, []string{"Show Name", "Another Title"}, []string{"de"}},
		{"The.French.Connection.1971.FRENCH.1080p.BluRay.x264-GRP", false, []string{"The French Connection"}, []string{"fr"}},
	}
	// A Source-sent language the name does not hold stays.
	n := "Show.Name.S01E01.The.German.Job.1080p.WEB.h264-GRP"
	if got := WithoutTitleWords(n, Hints{Show: true}, []string{"fr", "de"}, "The German Job"); !slices.Equal(got, []string{"fr"}) {
		t.Errorf("source languages: got %v, want [fr]", got)
	}
	for _, c := range cases {
		h := Hints{Show: c.show}
		p := Parse(c.name, h)
		got := WithoutTitleWords(c.name, h, p.Languages, c.titles...)
		if len(got) == 0 {
			got = nil
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: parse %v, got %v, want %v", c.name, p.Languages, got, c.want)
		}
	}
}
