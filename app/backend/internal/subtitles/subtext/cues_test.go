package subtext

import (
	"strings"
	"testing"
)

func TestParseCues_SRTAndBack(t *testing.T) {
	in := "1\r\n00:00:01,000 --> 00:00:02,500\r\n<i>Hello</i>\r\nthere\r\n\r\n2\r\n00:00:03,000 --> 00:00:04,000\r\nBye\r\n"
	cues, err := ParseCues(in, SRT)
	if err != nil || len(cues) != 2 || cues[0].Text != "<i>Hello</i>\nthere" || cues[0].StartMS != 1000 || cues[1].EndMS != 4000 {
		t.Fatalf("%+v %v", cues, err)
	}
	out := WriteSRT(cues)
	want := "1\n00:00:01,000 --> 00:00:02,500\n<i>Hello</i>\nthere\n\n2\n00:00:03,000 --> 00:00:04,000\nBye\n\n"
	if out != want {
		t.Fatalf("written %q", out)
	}
	again, err := ParseCues(out, SRT)
	if err != nil || len(again) != 2 || again[0] != cues[0] {
		t.Fatalf("round trip %+v %v", again, err)
	}
}

func TestParseCues_VTT(t *testing.T) {
	in := "WEBVTT\n\nNOTE made up\n\nintro\n00:01.000 --> 00:02.000 align:start\nOne\n\n00:00:03.000 --> 00:00:04.000\nTwo\n"
	cues, err := ParseCues(in, VTT)
	if err != nil || len(cues) != 2 || cues[0].Text != "One" || cues[1].StartMS != 3000 {
		t.Fatalf("%+v %v", cues, err)
	}
}

func TestParseCues_ASSKeepsStyleAndBreaks(t *testing.T) {
	in := "[Script Info]\nTitle: lab\n\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n" +
		"Dialogue: 0,0:00:05.00,0:00:06.50,Default,,0,0,0,,{\\an8\\i1}Up here{\\i0}\\Nnext, line\n" +
		"Comment: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,not shown\n" +
		"Dialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,First\\hword\n"
	cues, err := ParseCues(in, ASS)
	if err != nil || len(cues) != 2 {
		t.Fatalf("%+v %v", cues, err)
	}
	if cues[0].Text != "First word" || cues[1].Text != "<i>Up here</i>\nnext, line" || cues[1].EndMS != 6500 {
		t.Fatalf("%+v", cues)
	}
}

func TestParseCues_Refusals(t *testing.T) {
	if _, err := ParseCues("no cues here", SRT); err == nil {
		t.Error("text without cues was read")
	}
	if _, err := ParseCues("{1}{2}x", SUB); err == nil {
		t.Error("microdvd was read as cues")
	}
	if _, err := ParseCues(strings.Repeat("x", MaxText+1), SRT); err == nil {
		t.Error("oversized text was read")
	}
	// A Format line naming fewer fields than Start and End need is refused, not
	// read past its end.
	if _, err := ParseCues("Format:Text\nDialogue:0", ASS); err == nil {
		t.Error("a dialogue without its times was read")
	}
}

func TestWriteSRT_SkipsEmptyAndBlankLines(t *testing.T) {
	out := WriteSRT([]Cue{{StartMS: 0, EndMS: 1000, Text: " "}, {StartMS: 1000, EndMS: 2000, Text: "a\n\nb"}})
	if out != "1\n00:00:01,000 --> 00:00:02,000\na\nb\n\n" {
		t.Fatalf("%q", out)
	}
}

func FuzzParseCues(f *testing.F) {
	f.Add("1\n00:00:01,000 --> 00:00:02,000\nHi\n", SRT)
	f.Add("Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: 0,0:00:01.00,0:00:02.00,D,,0,0,0,,{\\i1x", ASS)
	f.Fuzz(func(t *testing.T, text, format string) {
		cues, err := ParseCues(text, format)
		if err == nil {
			_ = WriteSRT(cues)
		}
	})
}
