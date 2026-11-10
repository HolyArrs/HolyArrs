package releasename

import "testing"

// Library files named by the old apps: their quality word ("WEBDL-1080p")
// is quality, not a group; title words before a bracketed year and episode
// titles after " - " are never read as tags.
func TestOldAppLibraryNames(t *testing.T) {
	for _, c := range []struct {
		name   string
		show   bool
		res    int
		source string
		group  string
	}{
		{"The Matrix (1999) WEBDL-1080p", false, 1080, "web_dl", ""},
		{"The Matrix (1999) WEBDL-720p", false, 720, "web_dl", ""},
		{"The Matrix (1999) WEBRip-2160p", false, 2160, "web_rip", ""},
		{"The Matrix (1999) Bluray-720p", false, 720, "bluray", ""},
		{"The Matrix (1999) Remux-2160p", false, 2160, "remux", ""},
		{"The Matrix (1999) Bluray-1080p Proper", false, 1080, "bluray", ""},
		{"The Matrix (1999) HDTV-720p", false, 720, "tv", ""},
		{"Charlotte's Web (1973) Bluray-1080p", false, 1080, "bluray", ""},
		{"Cam (2018) WEBRip-1080p", false, 1080, "web_rip", ""},
		{"Gossip Girl - S01E03 - Charlotte's Web", true, 0, "", ""},
		{"Show - S01E05 - Cam Girl", true, 0, "", ""},
		{"Show - S01E02 - Title WEBDL-1080p", true, 1080, "web_dl", ""},
		{"Show (2018) - S01E02 - Title HDTV-720p", true, 720, "tv", ""},
		// Scene names keep reading as before.
		{"Show.S01E02.1080p.WEB-DL.DDP5.1.H.264-NTb", true, 1080, "web_dl", "NTb"},
		{"Movie.2020.1080p.BluRay.x264-SPARKS", false, 1080, "bluray", "SPARKS"},
		{"Movie.2020.720p.WEB-DL.DD5.1.H264-FGT", false, 720, "web_dl", "FGT"},
	} {
		r := Parse(c.name, Hints{Show: c.show})
		if r.Resolution.Value != c.res || r.Source.Value != c.source || r.Group.Value != c.group {
			t.Errorf("%q: got resolution %d, source %q, group %q; want %d, %q, %q", c.name, r.Resolution.Value, r.Source.Value, r.Group.Value, c.res, c.source, c.group)
		}
	}
}
