package downloads

import (
	"reflect"
	"testing"
)

func TestRootedFileNames(t *testing.T) {
	names := func(fs []File) []string {
		var out []string
		for _, f := range fs {
			out = append(out, f.Name)
		}
		return out
	}
	cases := []struct {
		name, save, content string
		in, want            []string
	}{
		{"folder torrent", "/data/torrents/movies", "/data/torrents/movies/Example.Movie.2025.1080p",
			[]string{"/Example.Movie.2025.1080p.mp4", "/Subs/English.srt"},
			[]string{"Example.Movie.2025.1080p/Example.Movie.2025.1080p.mp4", "Example.Movie.2025.1080p/Subs/English.srt"}},
		{"content is the save folder", "/data/torrents/movies", "/data/torrents/movies/",
			[]string{"/a.mkv"}, []string{"a.mkv"}},
		{"single file", "/data/t", "/data/t/a.mkv", []string{"/a.mkv"}, []string{"a.mkv"}},
		{"usual names untouched", "/data/t", "/data/t/Show", []string{"Show/a.mkv"}, []string{"Show/a.mkv"}},
		{"mixed names", "/data/t", "/data/t/Show", []string{"/a.mkv", "Show/b.mkv"}, []string{"Show/a.mkv", "Show/b.mkv"}},
		{"mixed names under content", "/data/t", "/data/t/Show", []string{"/x.S01E04.mkv", "x.S01E04.nfo"}, []string{"Show/x.S01E04.mkv", "Show/x.S01E04.nfo"}},
		{"mixed single-file content", "/data/t", "/data/t/a.mkv", []string{"/a.mkv", "a.nfo"}, []string{"a.mkv", "a.nfo"}},
		{"content outside save", "/data/t", "/etc", []string{"/passwd"}, []string{"/passwd"}},
		{"content climbs out", "/data/t", "/data/t/../x", []string{"/a.mkv"}, []string{"/a.mkv"}},
		{"no content path", "/data/t", "", []string{"/a.mkv"}, []string{"/a.mkv"}},
		{"climbing name kept for refusal", "/data/t", "/data/t/M", []string{"/../../etc/passwd"}, []string{"M/../../etc/passwd"}},
		{"drive letter name", "/data/t", "/data/t/M", []string{"/C:/x.mkv"}, []string{"M/C:/x.mkv"}},
		{"windows paths", `D:\t`, `D:\t\M`, []string{`\a.mkv`}, []string{"M/a.mkv"}},
	}
	for _, c := range cases {
		var in []File
		for _, n := range c.in {
			in = append(in, File{Name: n})
		}
		if got := names(rootedFileNames(in, c.save, c.content)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
