package mediafiles

import (
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

var musicLib = []TitleRef{
	{ID: 1, Name: "Velvet Cartography", Kind: "artist"},
	{ID: 2, Name: "The 1975 Signals", Kind: "artist"},
	{ID: 3, Name: "Jay-Rook", Kind: "artist"},
	{ID: 1, Name: "Northern Lanterns", Kind: "artist"},
	{ID: 4, Name: "Velvet Cartography", Year: 2019}, // a movie of the same name
}

// musicVerdicts are the music rows of release-names-testset.md under
// "Lossless first", for a 45-minute album with no copy yet.
var musicVerdicts = []struct {
	id      string
	size    int64 // MB
	outcome string
	step    string
	points  int
	rule    string // a failed reason code that must be there
}{
	{"U01", 400, "chosen", "lossless", 0, ""},
	{"U02", 1200, "chosen", "lossless_hires", 0, ""},
	{"U03", 400, "chosen", "lossless", 10, ""},
	{"U04", 100, "chosen", "lossy_high", 0, ""},
	{"U05", 100, "chosen", "lossy_high", 0, ""},
	{"U06", 40, "refused", "lossy_low", 0, "step.not_allowed"},
	{"U07", 2000, "chosen", "lossless_hires", -10, ""},
	{"U12", 400, "refused", "", 0, "hard.never:single_image"},
	{"U13", 100, "chosen", "lossy_high", 0, ""},
	{"U14", 60, "chosen", "lossy_high", 0, ""},
	{"U18", 400, "chosen", "lossless", 5, ""},
	{"U20", 80, "refused", "lossy_mid", 0, "step.not_allowed"},
	{"U22", 400, "chosen", "lossless", 5, ""},
	{"U23", 400, "refused", "", 0, "hard.safety:executable"},
	{"U01", 30, "refused", "lossless", 0, "hard.size"},
	{"U01", 9, "refused", "lossless", 0, "hard.safety:fake_by_size"},
}

// musicNames are the names of internal/parse/music_test.go (U01 to U30).
var musicNames = map[string]string{
	"U01": "Velvet Cartography - Low Tide Atlas (2019) [FLAC]",
	"U02": "Velvet Cartography - Low Tide Atlas (2019) [FLAC 24-96]",
	"U03": "Velvet_Cartography-Low_Tide_Atlas-CD-FLAC-2019-SPOOLHEAD",
	"U04": "Velvet Cartography - Low Tide Atlas (2019) [MP3 320]",
	"U05": "Velvet Cartography - Low Tide Atlas (2019) [MP3 V0]",
	"U06": "Velvet Cartography - Low Tide Atlas (2019) [128kbps]",
	"U07": "Velvet Cartography - Low Tide Atlas (2019) [Vinyl 24-192 FLAC]",
	"U08": "Velvet Cartography - Discography (2011-2024) [FLAC]",
	"U09": "Velvet Cartography - Harbour Songs (Deluxe Edition) (2021) [2CD] [FLAC]",
	"U10": "Velvet Cartography - Harbour Songs (2021) CD2 [FLAC]",
	"U11": "The 1975 Signals - Room Tone (2023) [FLAC]",
	"U12": "Velvet Cartography - Low Tide Atlas (2019) [FLAC] (image+cue)",
	"U13": "Velvet Cartography - Low Tide Atlas (2019) [AAC 256]",
	"U14": "Velvet Cartography - Low Tide Atlas (2019) [Opus 160]",
	"U15": "Velvet.Cartography-Low.Tide.Atlas-24BIT-WEB-FLAC-2019-SPOOLHEAD",
	"U16": "VA - Summer Static Vol 3 (2020) [MP3 320]",
	"U17": "Velvet Cartography - Low Tide Atlas (2019) [ALAC] {Remastered}",
	"U18": "Velvet Cartography - Low Tide Atlas (2019 Remaster) [WEB FLAC]",
	"U19": "Prism Hollow - 1999 (2001) [FLAC]",
	"U20": "Northern Lanterns - Live at Copper Hall (2018) [MP3 V2]",
	"U21": "Jay-Rook-Lowlands-WEB-2022-GRP",
	"U22": "Velvet Cartography - Low Tide Atlas (2019) [FLAC] [Log] [Cue]",
	"U23": "Velvet Cartography - Low Tide Atlas (2019) [FLAC].exe",
	"U24": "Velvet Cartography - Low Tide Atlas - 2019 - MP3 256kbps",
	"U25": "Velvet Cartography Low Tide Atlas 2019 FLAC",
	"U26": "Velvet Cartography - Low Tide Atlas (2019) [FLAC 16-44.1]",
	"U27": "Velvet Cartography - Low Tide Atlas (2019) [Japan Edition] [FLAC]",
	"U28": "Velvet Cartography - Low Tide Atlas (2019) [96kHz 24bit FLAC]",
	"U29": "Velvet Cartography - Low Tide Atlas (2019) [MP3 320] PROPER",
	"U30": "Velvet Cartography - Low Tide Atlas (Part One) (2019) [FLAC]",
}

func musicCase(t *testing.T, id string) string {
	n, ok := musicNames[id]
	if !ok {
		t.Fatalf("no case %s", id)
	}
	return n
}

func TestMusicVerdicts(t *testing.T) {
	rules := LosslessFirst()
	for _, c := range musicVerdicts {
		t.Run(c.id, func(t *testing.T) {
			name := musicCase(t, c.id)
			p := releasename.Parse(name, releasename.Hints{Music: true})
			rel := ReleaseFacts{Name: name, Parse: p, Size: c.size << 20, Protocol: "torrent", PublishedAt: time.Now().Add(-time.Hour)}
			m := MatchTitle(p, musicLib)
			if m.Title == nil || m.Title.ID != 1 {
				t.Fatalf("matched %+v", m)
			}
			v := Judge(rel, SlotSet{Match: m, SlotIDs: []int64{7}, RuntimeMinutes: 45, Out: true}, rules)
			if v.Outcome != c.outcome {
				t.Fatalf("outcome %s, want %s: %+v", v.Outcome, c.outcome, v.Reasons)
			}
			if c.step != "" && (v.Step == nil || v.Step.Name != c.step) {
				t.Errorf("step %+v, want %s", v.Step, c.step)
			}
			if v.Passed && v.Points != c.points {
				t.Errorf("points %d, want %d", v.Points, c.points)
			}
			if c.rule != "" {
				found := false
				for _, r := range v.Reasons {
					found = found || r.Code() == c.rule && r.Effect == "fail"
				}
				if !found {
					t.Errorf("no %s in %+v", c.rule, v.Reasons)
				}
			}
			for _, r := range v.Reasons {
				if r.Code() == "hard.language" && r.Effect != "pass" {
					t.Errorf("language checked for music: %+v", r)
				}
			}
		})
	}
}

func TestMusicGoodEnoughAndBetter(t *testing.T) {
	rules := LosslessFirst()
	mp3 := musicCase(t, "U04")
	flac := musicCase(t, "U01")
	if ReachesGoodEnough(mp3, rules) || !ReachesGoodEnough(flac, rules) {
		t.Fatal("good enough is lossless")
	}
	cur := CurrentOf(mp3, 0, time.Now(), rules)
	if cur.Step != "lossy_high" {
		t.Fatalf("current %+v", cur)
	}
	p := releasename.Parse(flac, releasename.Hints{Music: true})
	if !Gains(ReleaseFacts{Name: flac, Parse: p}, rules, &cur) {
		t.Fatal("lossless should replace a 320 copy")
	}
	low := releasename.Parse(mp3, releasename.Hints{Music: true})
	fl := CurrentOf(flac, 0, time.Now(), rules)
	if Gains(ReleaseFacts{Name: mp3, Parse: low}, rules, &fl) {
		t.Fatal("a 320 copy never replaces lossless")
	}
	if s, i := StepOf(flac, rules); s != "lossless" || i != 3 {
		t.Fatalf("step of %s %d", s, i)
	}
	// Video Pick rules cannot judge music and say why.
	v := Judge(ReleaseFacts{Name: flac, Parse: p, Size: 400 << 20}, SlotSet{Match: MatchTitle(p, musicLib), SlotIDs: []int64{1}, Out: true}, GoodEveryday())
	if v.Passed || !strings.Contains(v.Reasons[len(v.Reasons)-1].Detail+fmtReasons(v), "no music steps") {
		t.Fatalf("video rules judged music: %+v", v.Reasons)
	}
}

func fmtReasons(v Verdict) string {
	var b strings.Builder
	for _, r := range v.Reasons {
		b.WriteString(r.Detail + ";")
	}
	return b.String()
}

func TestMusicSteps(t *testing.T) {
	for in, want := range map[string]string{
		"A - B (2019) [MP3 192]": "lossy_mid", "A - B (2019) [MP3 V1]": "lossy_mid", "A - B (2019) [MP3]": "lossy_mid",
		"A - B (2019) [AAC 128]": "lossy_low", "A - B (2019) [Opus 128]": "lossy_low", "A - B (2019) [OGG 256]": "lossy_high",
		"A - B (2019) [WAV]": "lossless", "A - B (2019) [FLAC 16-48]": "lossless", "A - B (2019) [DSD]": "lossless_hires",
		"A - B (2019) [24bit]": "lossless_hires", "A - B (2019)": "", "A - B (2019) [Lossless]": "lossless",
	} {
		if got := StepFor(releasename.Parse(in, releasename.Hints{Music: true})); got != want {
			t.Errorf("%s: %q, want %q", in, got, want)
		}
	}
}

func TestMusicMatching(t *testing.T) {
	albums := []AlbumRef{{PartID: 10, Name: "Low Tide Atlas", Year: 2019}, {PartID: 11, Name: "Harbour Songs", Year: 2021},
		{PartID: 12, Name: "Echoes", Year: 2010}, {PartID: 13, Name: "Echoes", Year: 2020}, {PartID: 14, Name: "Lowlands", Year: 2022}}
	cases := []struct {
		name    string
		artist  int64
		part    int64
		warn    bool
		missing bool
	}{
		{"Velvet Cartography - Low Tide Atlas (2019) [FLAC]", 1, 10, false, false},
		{"velvet_cartography-low_tide_atlas-web-2019-grp", 1, 10, false, false},
		{"Velvet Cartography - Low Tide Atlas (2011 Remaster) [FLAC]", 1, 10, true, false},
		{"Velvet Cartography - Echoes (2020) [FLAC]", 1, 13, false, false},
		{"Velvet Cartography - Echoes (2021) [FLAC]", 1, 13, true, false},
		{"Velvet Cartography - Echoes [FLAC]", 1, 0, false, true},
		{"Velvet Cartography - Glass Ocean (2019) [FLAC]", 1, 0, false, true},
		{"Jay-Rook-Lowlands-WEB-2022-GRP", 3, 14, false, false},
		{"Velvet Cartography Harbour Songs 2021 FLAC", 1, 11, false, false},
	}
	for _, c := range cases {
		p := releasename.Parse(c.name, releasename.Hints{Music: true})
		m := MatchTitle(p, musicLib)
		if m.Title == nil || m.Title.ID != c.artist {
			t.Errorf("%s: artist %+v", c.name, m)
			continue
		}
		a, warn, why := MatchAlbum(p, m.Title.Name, albums)
		switch {
		case c.missing && (a != nil || why == ""):
			t.Errorf("%s: want no album, got %+v", c.name, a)
		case !c.missing && (a == nil || a.PartID != c.part):
			t.Errorf("%s: album %+v (%s), want %d", c.name, a, why, c.part)
		case (warn != "") != c.warn:
			t.Errorf("%s: warning %q", c.name, warn)
		}
	}
	// A music name never matches a movie, nor a movie name an artist.
	if m := MatchTitle(releasename.Parse("Velvet.Cartography.2019.1080p.WEB-DL-QUILL", releasename.Hints{}), musicLib); m.Title == nil || m.Title.ID != 4 {
		t.Errorf("movie matched %+v", m)
	}
	if m := MatchTitle(releasename.Parse("VA - Summer Static Vol 3 (2020) [MP3 320]", releasename.Hints{Music: true}), musicLib); m.Title != nil {
		t.Errorf("various artists matched %+v", m)
	}
}

// Same-named albums that share the year (or the near year) are refused with
// a reason that names the count, never matched to the first one.
func TestMusicMatchingSameYearRefused(t *testing.T) {
	cases := []struct {
		name   string
		albums []AlbumRef
	}{
		{"Velvet Cartography - Echoes (2020) [FLAC]", []AlbumRef{{PartID: 1, Name: "Echoes", Year: 2020}, {PartID: 2, Name: "Echoes", Year: 2020}}},
		// a studio album and a live one with the same name, released the same year
		{"Velvet Cartography - Echoes (2020) [FLAC]", []AlbumRef{{PartID: 1, Name: "Echoes", Year: 2020}, {PartID: 2, Name: "Echoes (Live)", Year: 2020},
			{PartID: 3, Name: "Echoes", Year: 2020}}},
		{"Velvet Cartography - Echoes (2021) [FLAC]", []AlbumRef{{PartID: 1, Name: "Echoes", Year: 2020}, {PartID: 2, Name: "Echoes", Year: 2022}}},
	}
	for _, c := range cases {
		p := releasename.Parse(c.name, releasename.Hints{Music: true})
		a, _, why := MatchAlbum(p, "Velvet Cartography", c.albums)
		if a != nil || !strings.Contains(why, "2 albums are called") {
			t.Errorf("%s: got %+v, why %q", c.name, a, why)
		}
	}
}

// An album whose title holds a safety word keeps the flag and is refused
// with a reason that says why, never dropped silently.
func TestMusicSafetyWordExplained(t *testing.T) {
	name := "Velvet Cartography - Password Songs (2019) [FLAC]"
	p := releasename.Parse(name, releasename.Hints{Music: true})
	v := Judge(ReleaseFacts{Name: name, Parse: p, Size: 400 << 20}, SlotSet{Match: MatchTitle(p, musicLib), SlotIDs: []int64{1}, RuntimeMinutes: 45, Out: true}, LosslessFirst())
	if v.Passed || !strings.Contains(fmtReasons(v), "archive needs a password") {
		t.Fatalf("%+v", v.Reasons)
	}
}
