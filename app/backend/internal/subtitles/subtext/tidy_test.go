package subtext

import (
	"fmt"
	"strings"
	"testing"
)

const dirty = `1
00:00:01,000 --> 00:00:03,000
Subtitles by ExampleTeam
www.example-subs.com

2
00:00:05,000 --> 00:00:07,500
JOHN: <i>[door slams]</i> lt's me.

3
00:00:07,000 --> 00:00:09,000
Where are you going  ?

4
00:00:10,000 --> 00:00:12,000
Nice 😀 day

5
00:00:13,000 --> 00:00:14,000
[music]

6
00:01:00,000 --> 00:01:02,000
Support us and become a VIP member
`

func TestTidyAll(t *testing.T) {
	out, rep, err := TidyText(dirty, SRT, Tidy{Ads: true, HI: true, Tags: true, Emoji: true, OCR: true, Spacing: true, Overlap: true, Language: "en"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Ads != 2 || len(rep.Removed) != 2 {
		t.Fatalf("ads %+v", rep)
	}
	for _, bad := range []string{"Subtitles by", "Support us", "<i>", "door", "JOHN", "😀", "[music]", "  "} {
		if strings.Contains(out, bad) {
			t.Fatalf("%q left in:\n%s", bad, out)
		}
	}
	// 84 ms (two frames at 23.976 fps) between cues that ran into each other.
	for _, good := range []string{"1\n00:00:05,000 --> 00:00:06,916\nIt's me.", "2\n00:00:07,000", "Where are you going?", "Nice day", "\n3\n"} {
		if !strings.Contains(out, good) {
			t.Fatalf("%q missing:\n%s", good, out)
		}
	}
	if strings.Contains(out, "\n4\n") {
		t.Fatalf("not renumbered:\n%s", out)
	}
}

func mkSRT(texts ...string) string {
	var sb strings.Builder
	for i, x := range texts {
		fmt.Fprintf(&sb, "%d\n00:00:%02d,000 --> 00:00:%02d,500\n%s\n\n", i+1, i*2+1, i*2+1, x)
	}
	return sb.String()
}

// Each row: the cue texts, the steps, the language, and text that must stay
// or go. These are the cases an earlier review found.
func TestTidyTable(t *testing.T) {
	filler := strings.Repeat("Just a normal line of dialogue here.\n", 1)
	var many []string
	for range 20 {
		many = append(many, strings.TrimSpace(filler))
	}
	withAt := func(i int, s string) []string {
		c := append([]string(nil), many...)
		c[i] = s
		return c
	}
	shout := make([]string, 20)
	for i := range shout {
		shout[i] = "WE NEED TO LEAVE RIGHT NOW BEFORE THEY COME."
	}
	shoutTime := append([]string(nil), shout...)
	shoutTime[8] = "MEET AT 10:30 TONIGHT"
	shoutName := append([]string(nil), shout...)
	shoutName[8] = "JOHN: GET DOWN!"
	cases := []struct {
		name string
		cues []string
		tidy Tidy
		keep []string
		drop []string
	}{
		{"clock time is not a speaker", withAt(8, "MEET AT 10:30 TONIGHT"), Tidy{HI: true}, []string{"MEET AT 10:30 TONIGHT"}, nil},
		{"speaker name goes", withAt(8, "JOHN: Get down!"), Tidy{HI: true}, []string{"Get down!"}, []string{"JOHN:"}},
		{"dash speaker keeps the dash", withAt(8, "- MARY: Run.\n- Why?"), Tidy{HI: true}, []string{"- Run.\n- Why?"}, []string{"MARY"}},
		{"OK is not a name", withAt(8, "OK: let's go."), Tidy{HI: true}, []string{"OK: let's go."}, nil},
		{"aside in round brackets stays", withAt(8, "I told him (and he agreed) to come."), Tidy{HI: true}, []string{"(and he agreed)"}, nil},
		{"capital note in round brackets goes", withAt(8, "(SIGHS) Fine."), Tidy{HI: true}, []string{"Fine."}, []string{"SIGHS"}},
		{"whole-line note in round brackets goes", withAt(8, "(door creaks)\nWho's there?"), Tidy{HI: true}, []string{"Who's there?"}, []string{"creaks"}},
		{"capitals file keeps speakers", shoutName, Tidy{HI: true}, []string{"JOHN: GET DOWN!"}, nil},
		{"capitals file keeps the clock", shoutTime, Tidy{HI: true, Caps: true}, []string{"Meet at 10:30 tonight"}, nil},
		{"one shouted line in a normal file stays", withAt(8, "GET OUT OF MY HOUSE!"), Tidy{Caps: true}, []string{"GET OUT OF MY HOUSE!"}, nil},
		{"capitals file is lowered", shout, Tidy{Caps: true}, []string{"We need to leave right now before they come."}, []string{"WE NEED"}},
		{"bare web address is dialogue", withAt(0, "Check google.com for it."), Tidy{Ads: true}, []string{"google.com"}, nil},
		{"web address with a call is an ad", withAt(0, "Visit www.best-subs.com"), Tidy{Ads: true}, nil, []string{"best-subs"}},
		{"site name is an ad", withAt(19, "Downloaded from opensubtitles"), Tidy{Ads: true}, nil, []string{"opensubtitles"}},
		{"German credit", withAt(0, "Untertitel von Klaus"), Tidy{Ads: true, Language: "de"}, nil, []string{"Klaus"}},
		{"Spanish credit", withAt(1, "Subtítulos por Pepe"), Tidy{Ads: true, Language: "es-MX"}, nil, []string{"Pepe"}},
		{"French credit", withAt(19, "Sous-titres par Amélie"), Tidy{Ads: true, Language: "fr"}, nil, []string{"Amélie"}},
		{"credit in the middle is dialogue", withAt(10, "Translated by my brother"), Tidy{Ads: true}, []string{"Translated by my brother"}, nil},
		{"German word on an English file stays", withAt(0, "Untertitel von Klaus"), Tidy{Ads: true, Language: "en"}, []string{"Klaus"}, nil},
		{"tags go", withAt(8, "<i>Hello</i> {\\an8}there"), Tidy{Tags: true}, []string{"Hello there"}, []string{"<i>"}},
		{"music note kept with emoji", withAt(8, "♪ la la ♪ 😀"), Tidy{Emoji: true}, []string{"♪ la la ♪"}, []string{"😀"}},
		{"French space before ? kept with spacing", withAt(8, "Quoi ?"), Tidy{Spacing: true, Language: "fr-CA"}, []string{"Quoi ?"}, nil},
		{"English space before ? goes with spacing", withAt(8, "What ?"), Tidy{Spacing: true, Language: "en"}, []string{"What?"}, nil},
		{"French space before ? kept without spacing", withAt(8, "Quoi ?"), Tidy{HI: true}, []string{"Quoi ?"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := mkSRT(c.cues...)
			out, _, err := TidyText(in, SRT, c.tidy)
			if err != nil {
				t.Fatal(err)
			}
			for _, k := range c.keep {
				if !strings.Contains(out, k) {
					t.Fatalf("%q lost:\n%s", k, out)
				}
			}
			for _, d := range c.drop {
				if strings.Contains(out, d) {
					t.Fatalf("%q left:\n%s", d, out)
				}
			}
		})
	}
}

func TestTidyOffAndOther(t *testing.T) {
	if out, rep, _ := TidyText(dirty, SRT, Tidy{}); out != dirty || rep.Did() {
		t.Fatal("no steps should change nothing")
	}
	out, _, _ := TidyText(dirty, SRT, Tidy{Ads: true, Language: "en"})
	if !strings.Contains(out, "[door slams]") || strings.Contains(out, "Subtitles by") {
		t.Fatal(out)
	}
	if out, _, _ := TidyText("[Script Info]", ASS, Tidy{Ads: true}); out != "[Script Info]" {
		t.Fatal(out)
	}
	// A second run changes nothing.
	once, _, _ := TidyText(dirty, SRT, Tidy{HI: true, Tags: true, Spacing: true, Overlap: true})
	if twice, rep, _ := TidyText(once, SRT, Tidy{HI: true, Tags: true, Spacing: true, Overlap: true}); twice != once || rep.Did() {
		t.Fatalf("second run changed:\n%s\n%s", once, twice)
	}
}

// Text that turns into a time line once its tags go keeps its text, so no
// blank cue is written (a case fuzzing found).
func TestTidyTimeLineInText(t *testing.T) {
	in := "1\n00:00:01,000 --> 00:00:02,000\n0:<B>00,000-->00:00,00\n\n2\n00:00:03,000 --> 00:00:04,000\nHi\n\n"
	out, _, err := TidyText(in, SRT, Tidy{Tags: true})
	if err != nil {
		t.Fatal(err)
	}
	cues, err := ParseCues(out, SRT)
	if err != nil || len(cues) != 2 {
		t.Fatalf("%v %d\n%s", err, len(cues), out)
	}
	for _, c := range cues {
		if strings.TrimSpace(c.Text) == "" {
			t.Fatalf("blank cue:\n%s", out)
		}
	}
}

func FuzzTidy(f *testing.F) {
	f.Add(dirty)
	f.Add("1\n00:00:01,000 --> 00:00:02,000\n0:<B>00,000-->00:00,00\n\n")
	f.Add(mkSRT("MEET AT 10:30", "(SIGHS)", "<i>a</i>\n\n<b>b</b>"))
	all := Tidy{Ads: true, HI: true, Tags: true, Emoji: true, OCR: true, Caps: true, Spacing: true, Overlap: true}
	f.Fuzz(func(t *testing.T, in string) {
		out, rep, err := TidyText(in, SRT, all)
		if err != nil || !rep.Did() {
			return
		}
		cues, err := ParseCues(out, SRT)
		if err != nil {
			t.Fatalf("cleaned file does not read: %v\n%q", err, out)
		}
		for _, c := range cues {
			if strings.TrimSpace(c.Text) == "" {
				t.Fatalf("blank cue in %q", out)
			}
		}
	})
}
