package arrapi

import (
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Cover names are read loosely. Only the two poster sizes Bazarr asks for
// make a smaller copy.
func TestCoverName_LooseNames(t *testing.T) {
	for file, want := range map[string]struct {
		kind string
		px   int
	}{"poster.jpg": {"poster", 0}, "poster-250.jpg": {"poster", 250}, "POSTER-500.PNG": {"poster", 500}, "poster-9999.jpg": {"poster", 0},
		"fanart.jpg": {"fanart", 0}, "fanart-360.jpg": {"fanart", 0}, "banner.jpg": {"banner", 0}, "poster": {"poster", 0}} {
		if k, px := coverName(file); k != want.kind || px != want.px {
			t.Errorf("coverName(%q) = %q, %d, want %q, %d", file, k, px, want.kind, want.px)
		}
	}
}

// outputPath is the download's own folder or file as HolyArrs sees it. It
// is the shared save folder only when nothing else is known.
func TestOutputPath_DownloadFolder(t *testing.T) {
	maps := []domain.PathMap{{Theirs: "/downloads/", Ours: "/data/downloads"}}
	seen := func(names ...string) []domain.SeenFile {
		var out []domain.SeenFile
		for _, n := range names {
			out = append(out, domain.SeenFile{Name: n})
		}
		return out
	}
	for _, c := range []struct {
		save string
		maps []domain.PathMap
		seen []domain.SeenFile
		want string
	}{
		{"/downloads/tv", maps, seen("Show.S01/e1.mkv", "Show.S01/sub/e1.srt"), "/data/downloads/tv/Show.S01"},
		{"/downloads/tv", maps, seen("Show.S01E01.mkv"), "/data/downloads/tv/Show.S01E01.mkv"},
		{"/downloads/tv", maps, seen(`Show.S01\e1.mkv`), "/data/downloads/tv/Show.S01"},
		{"/downloads/tv", maps, seen("a/x.mkv", "b/y.mkv"), "/data/downloads/tv"},
		{"/downloads/tv", maps, nil, "/data/downloads/tv"},
		{"/other/tv", maps, seen("Show.S01/e1.mkv"), "/other/tv/Show.S01"},
		{`D:\dl\tv`, nil, seen("Show.S01/e1.mkv"), `D:\dl\tv\Show.S01`},
		{"/downloads/tv", maps, seen("../x.mkv"), "/data/downloads/tv"},
		{"", maps, seen("x.mkv"), ""},
	} {
		if got := outputPath(c.save, c.maps, c.seen); got != c.want {
			t.Errorf("outputPath(%q, %v) = %q, want %q", c.save, c.seen, got, c.want)
		}
	}
}

// A file carries what Bazarr reads: languages by name, the quality name it
// splits on the dash, and the codecs. The parsed codec comes first, then
// the one read from the file.
func TestFileShape_BazarrFields(t *testing.T) {
	scene := "Lab.Show.S01E01.1080p.WEB-DL.H264-LAB"
	f := mediafiles.File{RelativePath: "Lab Show/Season 01/e1.mkv", Size: 5 << 20, AddedAt: time.Now(), ReleaseName: &scene,
		Quality: mediafiles.Quality{Resolution: 1080, Source: "web_dl", Audio: "AAC"}, Languages: []string{"en", "zz", "de"},
		MediaInfo: map[string]any{"video_codec": "h264"}}
	f.ID = 7
	mf := mediaFileOf(f, "/tv", "Lab Show")
	if mf.RelativePath != "Season 01/e1.mkv" || mf.SceneName != scene || mf.Size != 5<<20 {
		t.Errorf("file: %+v", mf)
	}
	if len(mf.Languages) != 2 || mf.Languages[0].Name != "English" || mf.Languages[1].Name != "German" {
		t.Errorf("languages: %+v", mf.Languages)
	}
	if q := mf.Quality["quality"].(map[string]any); q["name"] != "WEBDL-1080p" || q["resolution"] != 1080 {
		t.Errorf("quality: %v", q)
	}
	if mf.MediaInfo["videoCodec"] != "h264" || mf.MediaInfo["audioCodec"] != "AAC" || mf.MediaInfo["resolution"] != "1920x1080" {
		t.Errorf("media info: %v", mf.MediaInfo)
	}
	f.Quality.Codec, f.Quality.Audio, f.MediaInfo = "h265", "", map[string]any{}
	if mf := mediaFileOf(f, "/tv", "Lab Show"); mf.MediaInfo["videoCodec"] != "h265" || mf.MediaInfo["audioCodec"] != nil {
		t.Errorf("parsed codec first: %v", mf.MediaInfo)
	}
	for src, want := range map[string]string{"remux": "Bluray-1080p Remux", "disc_image": "BR-DISK-1080p", "tv": "HDTV-1080p", "odd": "odd-1080p"} {
		f.Quality.Source = src
		if q := mediaFileOf(f, "/tv", "Lab Show").Quality["quality"].(map[string]any); q["name"] != want {
			t.Errorf("%s: %v", src, q["name"])
		}
	}
	if l := originalLanguage(nil); l.Name != "Unknown" {
		t.Errorf("no original language: %+v", l)
	}
	if got := alternateTitles([]string{" Lab ", ""}); len(got) != 1 || got[0].Title != "Lab" {
		t.Errorf("alternate titles: %+v", got)
	}
}
