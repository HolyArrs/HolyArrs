package releasename

import (
	"slices"
	"testing"
)

// A written number beats UHD/4K, and HUN reads as Hungarian.
func TestIssue414_NumberBeatsUHD(t *testing.T) {
	r := Parse("The.Matrix.Reloaded.2003.720p.UHD.BluRay.DD-EX5.1 x264.HuN-SnyderHD", Hints{})
	if r.Resolution.Value != 720 || r.Resolution.Conf != Sure || len(r.Notes) != 0 {
		t.Fatalf("got %d (%v) notes %v", r.Resolution.Value, r.Resolution.Conf, r.Notes)
	}
	if r.Source.Value != "bluray" || !slices.Equal(r.Languages, []string{"hu"}) {
		t.Fatalf("source %q langs %v", r.Source.Value, r.Languages)
	}
	for name, want := range map[string]int{
		"Quiet.Harbour.2019.UHD.BluRay.x265-GROUP":       2160,
		"Quiet.Harbour.2019.4K.WEB.H265-GROUP":           2160,
		"Quiet.Harbour.2019.2160p.UHD.BluRay.x265-GROUP": 2160,
		"Quiet.Harbour.2019.1080p.4K.WEB.H264-GROUP":     1080,
	} {
		if got := Parse(name, Hints{}).Resolution.Value; got != want {
			t.Errorf("%s: %d, want %d", name, got, want)
		}
	}
}

func TestIssue414_Hungarian(t *testing.T) {
	for name, title := range map[string]string{
		"Matrix-Ujratoltve.2003.Open.Matte.WEBRIP.x264.HUN-nubira":              "Matrix-Ujratoltve",
		"The.Matrix.Reloaded.2003.Open.Matte.1080p.WEB-DL.DTS.x264.HuN-TRiNiTY": "The Matrix Reloaded",
		"The.Matrix.Reloaded.2003.BDRip.x264.HuN-ZHR":                           "The Matrix Reloaded",
		"The.Matrix.Reloaded.2003.720p.UHD.BluRay.DD-EX5.1 x264.HuN-SnyderHD":   "The Matrix Reloaded",
	} {
		r := Parse(name, Hints{})
		if !slices.Equal(r.Languages, []string{"hu"}) || r.Title.Value != title {
			t.Errorf("%s: langs %v title %q", name, r.Languages, r.Title.Value)
		}
	}
	// A title word stays a title word.
	if r := Parse("Corner.Diner.S01E03.The.Hun.720p.WEB.H264-GROUP", Hints{Show: true}); len(r.Languages) != 0 {
		t.Errorf("episode title Hun read as %v", r.Languages)
	}
	if r := Parse("Attila.the.Hun.2001.1080p.WEB.H264-GROUP", Hints{}); r.Title.Value != "Attila the Hun" || len(r.Languages) != 0 {
		t.Errorf("Attila: title %q langs %v", r.Title.Value, r.Languages)
	}
}
