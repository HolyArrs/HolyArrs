package releasename

import (
	"testing"
)

// One case per quality word: the word placed after a year (where short
// words like TC are read) and what it must read as.
func TestQualityTokens(t *testing.T) {
	cases := []struct {
		name, source string
		res          int
		codec, audio string
	}{
		{"Movie.2020.UHDBD.2160p.HEVC-GRP", "bluray", 2160, "h265", ""},
		{"Movie.2020.UHDBD.HEVC-GRP", "bluray", 2160, "h265", ""},
		{"Movie.2020.UHD-BD.2160.HEVC-GRP", "bluray", 2160, "h265", ""},
		{"Movie.2020.1080p.UHD.BluRay.x264-GRP", "bluray", 1080, "h264", ""},
		{"Movie.2020.1080p.WebHD.x264-GRP", "web_dl", 1080, "h264", ""},
		{"Movie.2020.1080p.Web-HD.x264-GRP", "web_dl", 1080, "h264", ""},
		// Remux joined by dashes.
		{"Movie.2020.Hybrid-Remux-2160p.HEVC-GRP", "remux", 2160, "h265", ""},
		{"Movie.2020.Remux-1080p.AVC-GRP", "remux", 1080, "h264", ""},
		// Web service words.
		{"Movie.2020.1080p.iTunesHD.x264-GRP", "web_dl", 1080, "h264", ""},
		{"Movie.2020.AmazonHD.x264-GRP", "web_dl", 0, "h264", ""},
		{"Movie.2020.NetflixUHD.x265-GRP", "web_dl", 2160, "h265", ""},
		{"Movie.2020.1080p.DisneyHD.x264-GRP", "web_dl", 1080, "h264", ""},
		{"Movie.2020.1080p.HBOMaxHD.x264-GRP", "web_dl", 1080, "h264", ""},
		{"Movie.2020.1080p.WEBMux.x264-GRP", "web_rip", 1080, "h264", ""},
		{"Movie.2020.HDRip.x264-GRP", "web_rip", 720, "h264", ""},
		// Discs.
		{"Movie.2020.1080p.HD-DVD.x264-GRP", "bluray", 1080, "h264", ""},
		{"Movie.2020.1080p.HDDVD.x264-GRP", "bluray", 1080, "h264", ""},
		{"Movie.2020.1080p.BDMux.x264-GRP", "bluray", 1080, "h264", ""},
		{"Movie.2020.1080p.BD25.AVC-GRP", "disc_image", 1080, "h264", ""},
		{"Movie.2020.2160p.BD100.HEVC-GRP", "disc_image", 2160, "h265", ""},
		{"Movie.2020.1080p.BRDISK-GRP", "disc_image", 1080, "", ""},
		{"Movie.2020.COMPLETE.UHD.BLURAY-GRP", "disc_image", 2160, "", ""},
		{"Movie.2020.COMPLETE.1080p.BluRay.x264-GRP", "bluray", 1080, "h264", ""},
		{"Show.S01.COMPLETE.1080p.BluRay.REMUX.AVC-GRP", "remux", 1080, "h264", ""},
		{"Movie.2020.BDRip.x264-GRP", "bluray", 480, "h264", ""},
		{"Movie.2020.BRRip.XviD-GRP", "bluray", 480, "xvid", ""},
		// TV.
		{"Movie.2020.SDTV.XviD-GRP", "tv", 480, "xvid", ""},
		{"Movie.2020.DSR.XviD-GRP", "tv", 480, "xvid", ""},
		{"Movie.2020.WS.DSR.XviD-GRP", "tv", 480, "xvid", ""},
		{"Movie.2020.TVRip.XviD-GRP", "tv", 480, "xvid", ""},
		{"Movie.2020.720p.HD.TV.x264-GRP", "tv", 720, "h264", ""},
		{"Movie.2020.1080i.RawHD.MPEG2-GRP", "tv", 1080, "mpeg2", ""},
		{"Movie.2020.UHDTV.HEVC-GRP", "tv", 2160, "h265", ""},
		{"Show.S01E01.HDTV.x264-GRP", "tv", 480, "h264", ""},
		// DVD.
		{"Movie.2020.DVDR-GRP", "dvd", 480, "mpeg2", ""},
		{"Movie.2020.PAL.DVD9-GRP", "dvd", 576, "mpeg2", ""},
		{"Movie.2020.NTSC.DVD5-GRP", "dvd", 480, "mpeg2", ""},
		{"Movie.2020.PAL.x264-GRP", "dvd", 576, "h264", ""},
		{"Movie.2020.DVD5.x264-GRP", "dvd", 480, "h264", ""},
		// Pre-release copies.
		{"Movie.2020.TC.XviD-GRP", "telecine", 0, "xvid", ""},
		{"Movie.2020.HDTC.x264-GRP", "telecine", 0, "h264", ""},
		{"Movie.2020.Telecine.XviD-GRP", "telecine", 0, "xvid", ""},
		{"Movie.2020.DVDSCR.XviD-GRP", "screener", 480, "xvid", ""},
		{"Movie.2020.SCR.XviD-GRP", "screener", 480, "xvid", ""},
		{"Movie.2020.Screener.XviD-GRP", "screener", 480, "xvid", ""},
		{"Movie.2020.R5.XviD-GRP", "regional", 480, "xvid", ""},
		{"Movie.2020.Workprint.XviD-GRP", "workprint", 0, "xvid", ""},
		{"Movie.2020.WP.XviD-GRP", "workprint", 0, "xvid", ""},
		{"Movie.2020.HDCAM.x264-GRP", "cam", 0, "h264", ""},
		{"Movie.2020.HDTS.x264-GRP", "telesync", 0, "h264", ""},
		// A real source wins over a pre-release word.
		{"Movie.2020.1080p.WEB-DL.TS.x264-GRP", "web_dl", 1080, "h264", ""},
		// Resolution forms.
		{"Movie.2020.1920x1080.WEB-DL-GRP", "web_dl", 1080, "", ""},
		{"Movie.2020.3840x2160.WEB-DL-GRP", "web_dl", 2160, "", ""},
		{"Movie.2020.1920x800.BluRay.x264-GRP", "bluray", 1080, "h264", ""},
		{"Movie.2020.1440p.WEB-DL.x265-GRP", "web_dl", 1440, "h265", ""},
		{"Movie.2020.FHD.WEB-DL-GRP", "web_dl", 1080, "", ""},
		{"Movie.2020.540p.WEBRip.x264-GRP", "web_rip", 540, "h264", ""},
		{"Movie.2020.360p.WEBRip.x264-GRP", "web_rip", 360, "h264", ""},
		{"Movie.2020.8K.WEB.HEVC-GRP", "web_dl", 4320, "h265", ""},
		{"Movie.2020.HD.WEBRip-GRP", "web_rip", 720, "", ""},
		{"[DB]Show_-_01_(BD1080p_x265).mkv", "bluray", 1080, "h265", ""},
		// Codecs.
		{"Movie.2020.1080p.WEB-DL.VP9-GRP", "web_dl", 1080, "vp9", ""},
		{"Movie.2020.DVDRip.DivX-GRP", "dvd", 480, "divx", ""},
		{"Movie.2020.1080p.BluRay.Hi10P.x264-GRP", "bluray", 1080, "h264", ""},
		// Audio.
		{"Movie.2020.2160p.BluRay.REMUX.HEVC.DTS-X-GRP", "remux", 2160, "h265", "dts_x"},
		{"Movie.2020.1080p.BluRay.x264.DTS-ES.6.1-GRP", "bluray", 1080, "h264", "dts_es"},
		{"Movie.2020.1080p.BluRay.x264.DTS-HD.HRA.7.1-GRP", "bluray", 1080, "h264", "dts_hd_hra"},
		{"Movie.2020.1080p.BluRay.x264.DTS-HD.MA.5.1-GRP", "bluray", 1080, "h264", "dts_hd_ma"},
		{"Movie.2020.1080p.BluRay.LPCM.1.0.x264-GRP", "bluray", 1080, "h264", "lpcm"},
		{"Movie.2020.1080p.BluRay.PCM.2.0.x264-GRP", "bluray", 1080, "h264", "lpcm"},
		{"Movie.2020.1080p.WEB-DL.HE-AAC.x264-GRP", "web_dl", 1080, "h264", "aac"},
		{"Movie.2020.1080p.WEB-DL.DDPA5.1.H264-GRP", "web_dl", 1080, "h264", "eac3"},
		{"Movie.2020.1080p.BluRay.TrueHD.7.1.AC3.x264-GRP", "bluray", 1080, "h264", "truehd"},
	}
	for _, c := range cases {
		r := Parse(c.name, Hints{})
		if r.Source.Value != c.source || r.Resolution.Value != c.res || r.Codec.Value != c.codec || r.Audio.Format != c.audio {
			t.Errorf("%s: got %s %d %q %q, want %s %d %q %q", c.name, r.Source.Value, r.Resolution.Value, r.Codec.Value,
				r.Audio.Format, c.source, c.res, c.codec, c.audio)
		}
	}
}

// Short source words are read only after the title, so titles keep them.
func TestQualityWordsInTitles(t *testing.T) {
	for name, title := range map[string]string{
		"TC.2019.1080p.WEB-DL-GRP":           "TC",
		"Real.Steel.2011.1080p.BluRay-GRP":   "Real Steel",
		"The.Pal.2019.1080p.WEB-DL-GRP":      "The Pal",
		"Screener.Story.2019.720p.WEB-GRP":   "Screener Story",
		"HD.Wave.2020.1080p.BluRay.x264-GRP": "HD Wave",
	} {
		if r := Parse(name, Hints{}); r.Title.Value != title {
			t.Errorf("%s: title %q, want %q", name, r.Title.Value, title)
		}
	}
}

// Fix numbers: REAL counts one more than the fix it repairs.
func TestFixWords(t *testing.T) {
	for name, fix := range map[string]int{
		"Movie.2020.1080p.BluRay.x264-GRP":             0,
		"Movie.2020.PROPER.1080p.BluRay.x264-GRP":      1,
		"Movie.2020.REAL.1080p.BluRay.x264-GRP":        1,
		"Movie.2020.PROPER.REAL.1080p.BluRay.x264-GRP": 2,
		"Movie.2020.REPACK2.1080p.BluRay.x264-GRP":     2,
	} {
		if r := Parse(name, Hints{}); r.Fix != fix {
			t.Errorf("%s: fix %d, want %d", name, r.Fix, fix)
		}
	}
}

// Group marks: reposting tools and site tags are not the group.
func TestGroupMarks(t *testing.T) {
	for name, group := range map[string]string{
		"Movie.2020.1080p.WEB-DL.x264-GRP-Obfuscated":    "GRP",
		"Movie.2020.1080p.WEB-DL.x264-GRP-Scrambled":     "GRP",
		"Movie.2020.1080p.WEB-DL.x264-GRP-xpost":         "GRP",
		"Movie.2020.1080p.BluRay.x264-GRP[rarbg]":        "GRP",
		"Show.S01E01.1080p.WEB.H264-GRP[eztv.re]":        "GRP",
		"Movie.2020.[1080p].[WEBRip].[YTS.MX]":           "YTS",
		"[Judas] Show - S01E03 [1080p][HEVC x265 10bit]": "Judas",
		"[Reza] Show [BD 1080p HEVC 10-bit FLAC]":        "Reza",
		"[SubsPlease] Show - 01 (1080p) [Multiple Subs]": "SubsPlease",
	} {
		if r := Parse(name, Hints{}); r.Group.Value != group {
			t.Errorf("%s: group %q, want %q", name, r.Group.Value, group)
		}
	}

}
