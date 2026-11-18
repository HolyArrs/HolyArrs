package subtext

import (
	"strings"
	"testing"
)

// cleanOne runs one cue through Cleanup and returns its text, or "<removed>".
func cleanOne(t *testing.T, text string, o CleanupOptions, lang string) string {
	t.Helper()
	out, _ := Cleanup([]Cue{{StartMS: 10000, EndMS: 11000, Text: text}}, o, lang)
	if len(out) == 0 {
		return "<removed>"
	}
	return out[0].Text
}

func TestCleanupSteps(t *testing.T) {
	tags := CleanupOptions{RemoveTags: true}
	emoji := CleanupOptions{RemoveEmoji: true}
	hi := CleanupOptions{RemoveHI: true}
	ocr := CleanupOptions{OCRFixes: true}
	common := CleanupOptions{CommonFixes: true}
	upper := CleanupOptions{FixUppercase: true}
	for _, c := range []struct {
		name, in string
		o        CleanupOptions
		lang     string
		want     string
	}{
		{"tags go", "<i>Hello</i> <b>there</b>", tags, "en", "Hello there"},
		{"font and position go", "{\\an8}<font color=\"#ff0000\">Up</font>", tags, "en", "Up"},
		{"unknown angle text stays", "a <3 b > c", tags, "en", "a <3 b > c"},
		{"emoji go", "Hi 😀 there 👍🏽", emoji, "en", "Hi there"},
		{"joined emoji go whole", "Family 👨‍👩‍👧 time", emoji, "en", "Family time"},
		{"music notes stay", "♪ La la ♪", emoji, "en", "♪ La la ♪"},
		{"sound in brackets goes", "[door slams]\nWho's there?", hi, "en", "Who's there?"},
		{"sound in parens goes", "(sighs) Fine.", hi, "en", "Fine."},
		{"description across lines goes", "[thunder\nrumbling]\nRun!", hi, "en", "Run!"},
		{"speaker goes, dash stays", "- JOHN: Hi.\n- MARY: Hey.", hi, "en", "- Hi.\n- Hey."},
		{"one speaker left loses dash", "- [gasps]\n- What?", hi, "en", "What?"},
		{"only sound: cue removed", "<i>[music playing]</i>", hi, "en", "<removed>"},
		{"tags of a dropped line move", "<i>[wind]\nHello</i>", hi, "en", "<i>Hello</i>"},
		{"mixed-case label stays", "Note: this stays.", hi, "en", "Note: this stays."},
		{"time stays", "Meet at 10:30.", hi, "en", "Meet at 10:30."},
		{"unclosed bracket stays", "a [b c", hi, "en", "a [b c"},
		{"lone l is I", "l think l'm lost.", ocr, "en", "I think I'm lost."},
		{"bar is I", "| know.", ocr, "en", "I know."},
		{"zero between letters", "G0D, it's c0ld.", ocr, "en", "GOD, it's cold."},
		{"numbers stay", "In 1990, 10 of 20.", ocr, "en", "In 1990, 10 of 20."},
		{"l stays outside English", "l'homme", ocr, "fr", "l'homme"},
		{"font colour stays", "<font color=\"#F0F0F0\">G0D</font>", ocr, "en", "<font color=\"#F0F0F0\">GOD</font>"},
		{"spaces", "Hello   there  .", common, "en", "Hello there."},
		{"comma space", "Yes,sir , ok", common, "en", "Yes, sir, ok"},
		{"thousands stay", "1,000 men", common, "en", "1,000 men"},
		{"ellipsis", "Wait….. no", common, "en", "Wait... no"},
		{"dialogue dash", "-Hi.\n--Hello.", common, "en", "- Hi.\n- Hello."},
		{"cut-off line dash stays", "--and then", common, "en", "--and then"},
		{"french space stays", "Quoi ? Non !", common, "fr", "Quoi ? Non !"},
		{"ellipsis space stays", "I said ...yes", common, "en", "I said ...yes"},
		{"uppercase", "I'M NOT SURE. ARE YOU?", upper, "en", "I'm not sure. Are you?"},
		{"uppercase over lines", "WE SHOULD GO\nRIGHT NOW.", upper, "en", "We should go\nright now."},
		{"short capitals stay", "FBI! Open up.", upper, "en", "FBI! Open up."},
		{"mixed line stays", "Hello THERE", upper, "en", "Hello THERE"},
		{"turkish i", "İSTANBUL ŞEHRİ", upper, "tr", "İstanbul şehri"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := cleanOne(t, c.in, c.o, c.lang); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestCleanupAds(t *testing.T) {
	cues := []Cue{
		{StartMS: 0, EndMS: 1000, Text: "Subtitles by <b>SomeTeam</b>"},
		{StartMS: 2000, EndMS: 3000, Text: "Hello."},
		{StartMS: 4000, EndMS: 5000, Text: "Where is it?"},
		{StartMS: 6000, EndMS: 7000, Text: "Over there."},
		{StartMS: 8000, EndMS: 9000, Text: "I read it on example.com."}, // middle: never checked
		{StartMS: 10000, EndMS: 11000, Text: "Fine."},
		{StartMS: 12000, EndMS: 13000, Text: "Bye."},
		{StartMS: 14000, EndMS: 15000, Text: "Bye now."},
		{StartMS: 16000, EndMS: 17000, Text: "Gone."},
		{StartMS: 18000, EndMS: 19000, Text: "Subtitles by the Amara.org community"},
	}
	out, rep := Cleanup(cues, CleanupOptions{RemoveAds: true}, "en")
	if len(out) != 8 || rep.Removed != 2 || rep.Steps["remove_ads"] != 2 || rep.Changed != 0 {
		t.Fatalf("%+v %+v", out, rep)
	}
	if out[0].Text != "Hello." || out[len(out)-1].Text != "Gone." {
		t.Fatalf("%+v", out)
	}
	for _, c := range []struct {
		text, lang string
		ad         bool
	}{
		{"Visit www.site.example", "en", true},
		{"Download at YTS.MX", "de", true},
		{"Synced and corrected by someone", "en", true},
		{"Synced and corrected by someone", "de", false}, // no German list yet
		{"The bus to Mr. Smith's.", "en", false},
		{"e.g. the nyts", "en", false},
		{"Support us and become a VIP member", "pt-br", false},
	} {
		got := adScore(c.text, adWords(c.lang)) >= adPass
		if got != c.ad {
			t.Errorf("%q in %s: ad %v", c.text, c.lang, got)
		}
	}
}

func TestRemoveAdsOnly(t *testing.T) {
	cues := []Cue{{StartMS: 0, EndMS: 1, Text: "www.ads.example"}, {StartMS: 2, EndMS: 3, Text: "[noise] Hi"}}
	out, n := RemoveAds(cues, "en")
	if n != 1 || len(out) != 1 || out[0].Text != "[noise] Hi" {
		t.Fatalf("%d %+v", n, out)
	}
	if same, n := RemoveAds(cues[1:], "en"); n != 0 || len(same) != 1 {
		t.Fatalf("%d %+v", n, same)
	}
}

func TestCleanupOverlaps(t *testing.T) {
	cues := []Cue{
		{StartMS: 1000, EndMS: 3000, Text: "a"},
		{StartMS: 2000, EndMS: 2500, Text: "b"}, // overlaps a
		{StartMS: 2500, EndMS: 4000, Text: "c"}, // touches b: left alone
		{StartMS: 4000, EndMS: 5000, Text: "top"},
		{StartMS: 4000, EndMS: 5000, Text: "bottom"}, // same start: on purpose
		{StartMS: 4020, EndMS: 6000, Text: "d"},      // closer than the gap
	}
	out, rep := Cleanup(cues, CleanupOptions{FixOverlaps: true}, "en")
	if out[0].EndMS != 2000-OverlapGapMS || out[1].EndMS != 2500 || out[2].EndMS != 4000 {
		t.Fatalf("%+v", out)
	}
	if out[3].EndMS != 5000 || out[4].EndMS != 4020 {
		t.Fatalf("%+v", out)
	}
	if rep.Steps["fix_overlaps"] != 2 || rep.Changed != 2 {
		t.Fatalf("%+v", rep)
	}
	for _, c := range out {
		if c.EndMS <= c.StartMS {
			t.Fatalf("empty cue %+v", c)
		}
	}
}

const dirty = "1\n00:00:01,000 --> 00:00:02,000\nSubtitles by Team X\n\n" +
	"2\n00:00:03,000 --> 00:00:04,000\n<i>[door opens]</i>\n\n" +
	"3\n00:00:05,000 --> 00:00:06,000\n- JOHN: Hi , l'm here.\n- [laughs]\n\n" +
	"4\n00:00:07,000 --> 00:00:08,000\nGood.\n"

func TestCleanupSRT(t *testing.T) {
	all := CleanupOptions{RemoveHI: true, RemoveTags: true, RemoveEmoji: true, OCRFixes: true, CommonFixes: true, FixUppercase: true, RemoveAds: true, FixOverlaps: true}
	out, rep, err := CleanupSRT(dirty, all, "en")
	if err != nil {
		t.Fatal(err)
	}
	want := "1\n00:00:05,000 --> 00:00:06,000\nHi, I'm here.\n\n2\n00:00:07,000 --> 00:00:08,000\nGood.\n\n"
	if out != want {
		t.Fatalf("got %q", out)
	}
	if rep.Removed != 2 || rep.Changed != 1 {
		t.Fatalf("%+v", rep)
	}
	// Cleaning again finds nothing more and hands back the same text.
	again, rep2, err := CleanupSRT(out, all, "en")
	if err != nil || again != out || rep2.Any() {
		t.Fatalf("second run %q %+v %v", again, rep2, err)
	}
}

func TestCleanupSRTLeavesTextAlone(t *testing.T) {
	// CRLF and odd numbering would change on a rewrite, so they show whether
	// the text came back untouched.
	in := "7\r\n00:00:01,000 --> 00:00:02,000\r\nHello.\r\n\r\n9\r\n00:00:03,000 --> 00:00:04,000\r\nBye.\r\n"
	all := CleanupOptions{RemoveHI: true, RemoveTags: true, RemoveEmoji: true, OCRFixes: true, CommonFixes: true, FixUppercase: true, RemoveAds: true, FixOverlaps: true}
	for name, o := range map[string]CleanupOptions{"all off": {}, "nothing to do": all} {
		out, rep, err := CleanupSRT(in, o, "en")
		if err != nil || out != in || rep.Any() {
			t.Errorf("%s: %q %+v %v", name, out, rep, err)
		}
	}
	if out, _, _ := CleanupSRT(dirty, CleanupOptions{}, "en"); out != dirty {
		t.Error("all off changed the text")
	}
}

func TestCleanupSRTRefusesLooseText(t *testing.T) {
	in := "1\n00:00:01,000 --> 00:00:02,000\nFirst part\n\nlost after a blank line\n\n2\n00:00:03,000 --> 00:00:04,000\n[noise]\n"
	out, _, err := CleanupSRT(in, CleanupOptions{RemoveHI: true}, "en")
	if err != ErrLooseText || out != in {
		t.Fatalf("%q %v", out, err)
	}
	if out, _, err := CleanupSRT("not a subtitle", CleanupOptions{RemoveHI: true}, "en"); err == nil || out != "not a subtitle" {
		t.Fatalf("%q %v", out, err)
	}
}

func TestCleanupLinearOnHostileText(t *testing.T) {
	// Opens with no close, and tags with no end, must stay cheap.
	big := strings.Repeat("[", 1<<20) + strings.Repeat("<i", 1<<18) + strings.Repeat("{\\", 1<<18)
	all := CleanupOptions{RemoveHI: true, RemoveTags: true, RemoveEmoji: true, OCRFixes: true, CommonFixes: true, FixUppercase: true, RemoveAds: true, FixOverlaps: true}
	_, _ = Cleanup([]Cue{{StartMS: 0, EndMS: 1, Text: big}}, all, "en")
}

func FuzzCleanup(f *testing.F) {
	f.Add(dirty, "en")
	f.Add("1\n00:00:01,000 --> 00:00:02,000\n[a (b] c) <i>- X: 0\n", "fr")
	f.Fuzz(func(t *testing.T, text, lang string) {
		all := CleanupOptions{RemoveHI: true, RemoveTags: true, RemoveEmoji: true, OCRFixes: true, CommonFixes: true, FixUppercase: true, RemoveAds: true, FixOverlaps: true}
		out, rep, err := CleanupSRT(text, all, lang)
		if err != nil || !rep.Any() {
			if out != text {
				t.Fatal("text changed without a cleanup")
			}
			return
		}
		cues, err := ParseCues(out, SRT)
		if err != nil && out != "" {
			t.Fatalf("cleaned text does not read back: %v", err)
		}
		for _, c := range cues {
			if blank(c.Text) {
				t.Fatalf("blank cue written: %q", c.Text)
			}
		}
	})
}
