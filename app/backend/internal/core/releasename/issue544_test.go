package releasename

import (
	"slices"
	"testing"
)

// French release names tag the dub; each tag reads as its version,
// and plain FRENCH, VF or MULTI say none.
func TestFrenchVersion544(t *testing.T) {
	for _, c := range []struct{ name, want string }{
		{"Movie.2024.VFF.1080p.WEB.H264-GRP", "VFF"},
		{"Movie.2024.TRUEFRENCH.1080p.BluRay.x264-GRP", "VFF"},
		{"Movie.2024.MULTI.VFF.2160p.WEB-DL-GRP", "VFF"},
		{"Movie.2024.MULTi.TRUEFRENCH.1080p.WEB-GRP", "VFF"},
		{"Movie.2024.FRENCH.CA.1080p.WEB-GRP", "VFQ"},
		{"Movie 2024 FRENCH-CA 1080p", "VFQ"},
		{"Movie.2024.MULTI.CA.1080p.WEB-GRP", "VFQ"},
		{"Movie.2024.MULTI.VFQ.1080p.WEB-GRP", "VFQ"},
		{"Movie.2024.VFQ.720p.HDTV-GRP", "VFQ"},
		{"Movie.2024.MULTi.VFI.1080p.WEB-GRP", "VFI"},
		{"Movie.2024.MULTi.VF2.1080p.BluRay-GRP", "VF2"},
		{"Movie.2024.MULTi.VFF.VFQ.1080p.BluRay-GRP", "VF2"},
		{"Movie.2024.FRENCH.1080p.WEB-GRP", ""},
		{"Movie.2024.MULTI.1080p.WEB-GRP", ""},
		{"Movie.2024.VF.1080p.WEB-GRP", ""},
		{"Movie.2024.CATALAN.CA.1080p.WEB-GRP", ""},
		{"Movie.2024.1080p.WEB.CA-GRP", ""},
		{"Vffy.2024.1080p.WEB-GRP", ""},
		{"Show.S01E01.MULTi.VFQ.1080p.WEB-GRP", "VFQ"},
	} {
		if got := FrenchVersion(c.name); got != c.want {
			t.Errorf("FrenchVersion(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

// Every French tag still reads French, and MULTI.CA is French too.
func TestFrenchTagsReadFrench544(t *testing.T) {
	for _, n := range []string{"Movie.2024.FRENCH.CA.1080p.WEB-GRP", "Movie.2024.VFQ.1080p.WEB-GRP", "Movie.2024.TRUEFRENCH.1080p.WEB-GRP",
		"Movie.2024.VFI.1080p.WEB-GRP", "Movie.2024.VF2.1080p.WEB-GRP", "Movie.2024.MULTi.CA.1080p.WEB-GRP", "Movie 2024 MULTI-CA 1080p"} {
		if l := Parse(n, Hints{}).Languages; !slices.Contains(l, "fr") {
			t.Errorf("%s: languages %v", n, l)
		}
	}
}

func TestFrenchVersionOfTrack544(t *testing.T) {
	for _, c := range []struct{ lang, name, want string }{
		{"fr-CA", "", "VFQ"},
		{"fr-FR", "", "VFF"},
		{"fre", "French (CA)", "VFQ"},
		{"fre", "VFQ 5.1", "VFQ"},
		{"fr", "Canada", "VFQ"},
		{"fr", "Québec", "VFQ"},
		{"fr", "France", "VFF"},
		{"fra", "TrueFrench", "VFF"},
		{"fr", "VFI", "VFI"},
		{"fr", "", ""},
		{"eng", "English", ""},
		{"fy", "Frisian", ""},
		{"ca", "Catalan", ""},
		{"", "Français VFQ", "VFQ"},
	} {
		if got := FrenchVersionOfTrack(c.lang, c.name); got != c.want {
			t.Errorf("track(%q, %q) = %q, want %q", c.lang, c.name, got, c.want)
		}
	}
	if got := FrenchVersionOfTracks("VFF", "", "VFQ"); got != "VF2" {
		t.Errorf("both tracks = %q", got)
	}
}

func TestFrenchVersionOfNFO544(t *testing.T) {
	nfo := "General\r\nComplete name : Movie.mkv\r\n\r\nVideo\r\nFormat : HEVC\r\n\r\n" +
		"Audio #1\r\nFormat : E-AC-3\r\nTitle : English\r\nLanguage : English\r\n\r\n" +
		"Audio #2\r\nFormat : AC-3\r\nTitle : VFQ\r\nLanguage : French\r\n\r\n" +
		"Text #1\r\nTitle : France\r\nLanguage : French\r\n"
	if got := FrenchVersionOfNFO(nfo); got != "VFQ" {
		t.Errorf("nfo = %q", got)
	}
	if got := FrenchVersionOfNFO("Audio\nLanguage.......: French (CA)\n"); got != "VFQ" {
		t.Errorf("dotted nfo = %q", got)
	}
	// A French subtitle never names the dub.
	if got := FrenchVersionOfNFO("Text #1\nTitle : VFF\nLanguage : French\n"); got != "" {
		t.Errorf("subtitle-only nfo = %q", got)
	}
	if got := FrenchVersionOfNFO("Greetz to all\nRelease notes : enjoy\n"); got != "" {
		t.Errorf("no mediainfo = %q", got)
	}
}

// CA alone stays Catalan, not French.
func TestCatalanNotFrench544(t *testing.T) {
	if l := Parse("Movie.2024.CATALAN.CA.1080p.WEB-GRP", Hints{}).Languages; slices.Contains(l, "fr") {
		t.Errorf("languages %v", l)
	}
}
