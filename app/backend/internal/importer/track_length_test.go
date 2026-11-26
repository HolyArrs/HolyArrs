package importer

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/importer/tags"
)

// The play length as a second opinion on a track match.
func TestTrackLengthSignal(t *testing.T) {
	al := &musicAlbum{tracks: []musicTrack{
		{disc: 1, number: 1, name: "Alpha", length: 200000, slot: 1},
		{disc: 1, number: 2, name: "Beta", length: 180000, slot: 2},
		{disc: 2, number: 1, name: "Gamma", length: 120000, slot: 3},
		{disc: 2, number: 2, name: "Delta", length: 0, slot: 4},
	}}
	cases := []struct {
		name string
		path string
		ms   int
		tg   tags.Tags
		want string // track name, "" for none
		sure bool
	}{
		{"number and title agree", "a.flac", 0, tags.Tags{Track: 1, Disc: 1, Title: "Alpha"}, "Alpha", true},
		{"title differs, length agrees", "a.flac", 201500, tags.Tags{Track: 1, Disc: 1, Title: "Alfa"}, "Alpha", true},
		{"title differs, length 3.5 s off", "a.flac", 203500, tags.Tags{Track: 1, Disc: 1, Title: "Alfa"}, "Alpha", false},
		{"live take, length agrees", "a.flac", 200000, tags.Tags{Track: 1, Disc: 1, Title: "Alpha (Live)"}, "Alpha", false},
		{"no title, length 19 s off", "a.flac", 199000, tags.Tags{Track: 2, Disc: 1}, "Beta", false},
		{"no title, number fits, length unknown", "a.flac", 0, tags.Tags{Track: 2, Disc: 1}, "Beta", true},
		{"no title, length far off", "a.flac", 60000, tags.Tags{Track: 2, Disc: 1}, "Beta", false},
		{"no disc, counted across, length agrees", "x.flac", 120900, tags.Tags{Track: 3}, "Gamma", true},
		{"no disc, counted across, length unknown", "x.flac", 0, tags.Tags{Track: 3}, "Gamma", false},
		{"no disc, counted across, track has no length", "x.flac", 90000, tags.Tags{Track: 4}, "Delta", false},
		{"featured title", "a.flac", 0, tags.Tags{Track: 1, Disc: 1, Title: "Alpha (feat. Guest)"}, "Alpha", true},
		{"untagged file name, length agrees", "01 - Alpha.flac", 200000, tags.Tags{}, "Alpha", true},
		{"untagged file name, length far off", "01 - Alpha.flac", 30000, tags.Tags{}, "Alpha", false},
		{"title only, length far off", "a.flac", 30000, tags.Tags{Track: 9, Disc: 1, Title: "Beta"}, "Beta", false},
	}
	for _, c := range cases {
		f := &ByHandFile{Path: c.path, Size: 1000, lengthMS: c.ms, lengthRead: true}
		tr, sure := al.track(f, c.tg)
		got := ""
		if tr != nil {
			got = tr.name
		}
		if got != c.want || sure != c.sure {
			t.Errorf("%s: %q sure=%v, want %q sure=%v", c.name, got, sure, c.want, c.sure)
		}
	}
}
