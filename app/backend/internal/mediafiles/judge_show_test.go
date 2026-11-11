package mediafiles

import (
	"slices"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// The show part of the fixed library in release-names-testset.md, next to
// the movie Harbor of Glass (2018).
var showLibrary = append(slices.Clone(library),
	TitleRef{ID: 20, Name: "Copper Valley", Year: 2015, OriginalLanguage: "en", Runtime: 45, Kind: "show"},
	TitleRef{ID: 21, Name: "Harbor of Glass", Year: 2019, OriginalLanguage: "en", Runtime: 45, Kind: "show"},
	TitleRef{ID: 22, Name: "The Late Lamp Show", Year: 2010, OriginalLanguage: "en", Runtime: 45, Kind: "show"},
	TitleRef{ID: 23, Name: "Glass Orchard", Year: 2022, OriginalLanguage: "en", Runtime: 45, Kind: "show"},
	TitleRef{ID: 24, Name: "Unit 24", Year: 2008, OriginalLanguage: "en", Runtime: 45, Kind: "show"},
	TitleRef{ID: 25, Name: "9 Lives of Mara Penn", Year: 2020, OriginalLanguage: "en", Runtime: 45, Kind: "show"},
	TitleRef{ID: 26, Name: "The Hundred Doors", Year: 2016, OriginalLanguage: "en", Runtime: 45, Kind: "show"},
	TitleRef{ID: 27, Name: "Starfall Kitchen", Year: 2012, OriginalLanguage: "ja", Runtime: 24, Kind: "show"},
	TitleRef{ID: 28, Name: "Wings of the Wind", Year: 2021, OriginalLanguage: "ja", Runtime: 24, Kind: "show", Aliases: []string{"Kaze no Tsubasa", "風の翼"}},
)

// episodes builds a show's episodes: seasons x per season, absolute
// numbers counted from 1, air days one a week from 1 January 2026.
func episodes(seasons, per int) []EpisodeRef {
	var out []EpisodeRef
	abs := 0
	day := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for s := 0; s <= seasons; s++ {
		for e := 1; e <= per; e++ {
			r := EpisodeRef{ItemID: int64(s*100 + e), Season: s, Episode: e, AirDay: day.Format("2006-01-02")}
			if s > 0 {
				abs++
				r.Absolute = abs
			}
			out = append(out, r)
			day = day.AddDate(0, 0, 7)
		}
	}
	return out
}

type showRow struct {
	ID      string
	Name    string
	Size    int64
	Outcome string
	Points  int
	Codes   []string
	Title   int64
	Eps     []int64 // episode item ids covered (checked when set)
}

var showRows = []showRow{
	{ID: "S01", Name: "Copper.Valley.S01E01.1080p.WEB-DL.DDP5.1.H.264-QUILL", Outcome: "chosen", Points: 10, Title: 20, Eps: []int64{101}},
	{ID: "S02", Name: "Copper.Valley.S01E02.720p.HDTV.x264-OXBOW", Outcome: "chosen", Points: 0, Title: 20, Eps: []int64{102}},
	{ID: "S03", Name: "copper_valley_1x03_hdtv_x264-oxbow", Outcome: "refused", Codes: []string{"step.not_allowed"}, Title: 20, Eps: []int64{103}},
	{ID: "S04", Name: "Copper.Valley.S01E04E05.1080p.WEB.h264-MOSSFOX", Outcome: "chosen", Points: 0, Title: 20, Eps: []int64{104, 105}},
	{ID: "S05", Name: "Copper.Valley.S01E06-E08.1080p.WEB-DL.H.265-QUILL", Outcome: "chosen", Points: 15, Title: 20, Eps: []int64{106, 107, 108}},
	{ID: "S06", Name: "Copper.Valley.S01.1080p.BluRay.x265.DDP5.1-LARKSPUR", Outcome: "chosen", Points: 15, Title: 20},
	{ID: "S08", Name: "Copper.Valley.S00E03.Behind.the.Valley.1080p.WEB-DL-QUILL", Outcome: "chosen", Points: 5, Title: 20, Eps: []int64{3}},
	{ID: "S09", Name: "Harbor.of.Glass.2019.S02E01.1080p.WEB-DL.H.264-QUILL", Outcome: "chosen", Points: 5, Title: 21, Eps: []int64{201}},
	{ID: "S11", Name: "The.Late.Lamp.Show.2026-01-08.1080p.WEB-DL.x264-NIGHTDESK", Outcome: "chosen", Points: 5, Title: 22, Eps: []int64{2}},
	{ID: "S13", Name: "Glass.Orchard.Part.2.1080p.WEB-DL.H.264-QUILL", Outcome: "chosen", Points: 5, Title: 23, Eps: []int64{102}},
	{ID: "S15", Name: "Unit.24.S03E02.1080p.WEB-DL.H.265-MOSSFOX", Outcome: "chosen", Points: 15, Title: 24, Eps: []int64{302}},
	{ID: "S19", Name: "Copper.Valley.S02E10.1080p.WEB-DL.H.264.FRENCH-LESQUAIS", Outcome: "refused", Codes: []string{"hard.language"}, Title: 20},
	{ID: "S20", Name: "Copper.Valley.S02E10.MULTi.1080p.WEB-DL.H.264-LESQUAIS", Outcome: "chosen", Points: 5, Title: 20},
	{ID: "S21", Name: "Copper.Valley.S02E11.1080p.WEB-DL.H.264.HC.KORSUB-GRAVELY", Outcome: "refused", Codes: []string{"hard.never:hardcoded_subs"}, Title: 20},
	{ID: "S23", Name: "Copper Valley - 2x13 - The Dam Breaks [1080p]", Outcome: "refused", Codes: []string{"step.none"}, Title: 20},
	{ID: "S24", Name: "Copper.Valley.S03E01.2160p.WEB-DL.DV.H.265-QUILL", Outcome: "refused", Codes: []string{"step.not_allowed"}, Title: 20},
	{ID: "S25", Name: "9.Lives.of.Mara.Penn.S01E09.720p.WEB-DL.H.264-MOSSFOX", Outcome: "chosen", Points: 5, Title: 25},
	{ID: "S27", Name: "Copper.Valley.S01E01.Sample.1080p.WEB-DL-QUILL", Size: 45 * mb, Outcome: "refused", Codes: []string{"hard.safety:sample"}, Title: 20},
	{ID: "S99", Name: "Copper.Valley.S09E01.1080p.WEB-DL.H.264-QUILL", Outcome: "refused", Codes: []string{"match.episode"}, Title: 20},
	{ID: "A01", Name: "[Hoshizora] Starfall Kitchen - 12 [1080p]", Outcome: "chosen", Points: 0, Title: 27, Eps: []int64{202}},
	{ID: "A03", Name: "[Kumomaru] Starfall Kitchen - 13 (1080p WEB-DL AAC) [A1B2C3D4]", Outcome: "chosen", Points: 5, Title: 27, Eps: []int64{203}},
	{ID: "A05", Name: "[Hoshizora] Starfall Kitchen - 11-15 [1080p] (Batch)", Size: 5 * gb, Outcome: "chosen", Points: 0, Title: 27, Eps: []int64{201, 202, 203, 204, 205}},
	{ID: "A08", Name: "[Hoshizora] Kaze no Tsubasa - 03 [1080p][Dual Audio]", Outcome: "chosen", Points: 0, Title: 28, Eps: []int64{103}},
	{ID: "A09", Name: "[Kumomaru] 風の翼 - 04 [1080p]", Outcome: "chosen", Points: 0, Title: 28, Eps: []int64{104}},
	{ID: "A10", Name: "[Hoshizora] Starfall Kitchen - 12 [1080p][HardSub]", Outcome: "refused", Codes: []string{"hard.never:hardcoded_subs"}, Title: 27},
	{ID: "A11", Name: "Starfall Kitchen - 1080 [1080p]", Outcome: "refused", Codes: []string{"match.episode"}, Title: 27},
	{ID: "A14", Name: "[Hoshizora] Starfall Kitchen - 12 [480p]", Outcome: "refused", Codes: []string{"step.not_allowed"}, Title: 27},
}

func showEpisodes(id int64) []EpisodeRef {
	switch id {
	case 27, 28:
		return episodes(3, 10)
	case 22:
		return episodes(0, 20) // a daily show: every episode in season 0 here, matched by air day
	}
	return episodes(3, 13)
}

func TestShowVerdicts(t *testing.T) {
	rules := GoodEveryday()
	for _, r := range showRows {
		t.Run(r.ID, func(t *testing.T) {
			p := releasename.Parse(r.Name, releasename.Hints{Size: r.Size, Show: true})
			m := MatchTitle(p, showLibrary)
			if m.Title == nil || m.Title.ID != r.Title {
				t.Fatalf("matched %+v (%s), want title %d", m.Title, m.Detail, r.Title)
			}
			eps, why := MatchEpisodes(p, "aired", showEpisodes(m.Title.ID))
			set := SlotSet{Match: m, Out: true, MinSeeders: ptr(1), RuntimeMinutes: 45 * max(len(eps), 1)}
			if why != "" {
				set.MatchReasons = []Reason{{Step: "match", Rule: "episode", Effect: "fail", Detail: why}}
			} else {
				set.MatchReasons = []Reason{{Step: "match", Rule: "episode", Effect: "pass", Detail: "episodes found"}}
				for _, e := range eps {
					set.SlotIDs = append(set.SlotIDs, e.ItemID)
				}
			}
			if r.Eps != nil && !slices.Equal(set.SlotIDs, r.Eps) {
				t.Errorf("episodes %v, want %v (%s)", set.SlotIDs, r.Eps, why)
			}
			size := r.Size
			if size == 0 {
				size = int64(len(set.SlotIDs)) * 1200 * mb
				if p.Resolution.Value == 720 {
					size /= 2
				}
				if size == 0 {
					size = 1200 * mb
				}
			}
			rel := ReleaseFacts{Name: r.Name, Parse: p, Size: size, Protocol: "torrent", Seeders: ptr(85), PublishedAt: time.Now().Add(-6 * time.Hour)}
			v := Judge(rel, set, rules)
			if v.Outcome != r.Outcome {
				t.Fatalf("outcome %s, want %s:\n%s", v.Outcome, r.Outcome, dump(v))
			}
			if v.Passed && v.Points != r.Points {
				t.Errorf("points %d, want %d\n%s", v.Points, r.Points, dump(v))
			}
			fails := codes(v, "fail")
			for _, c := range r.Codes {
				if !slices.Contains(fails, c) {
					t.Errorf("missing refusal %s; got %v", c, fails)
				}
			}
		})
	}
}

func TestEpisodeNameNeverMatchesMovie(t *testing.T) {
	p := releasename.Parse("Harbor.of.Glass.S02E01.1080p.WEB-DL.H.264-QUILL", releasename.Hints{})
	if m := MatchTitle(p, showLibrary); m.Title == nil || m.Title.ID != 21 {
		t.Fatalf("matched %+v, want the show", m.Title)
	}
	p = releasename.Parse("Harbor.of.Glass.2018.1080p.WEB-DL.H.264-QUILL", releasename.Hints{})
	if m := MatchTitle(p, showLibrary); m.Title == nil || m.Title.ID != 2 {
		t.Fatalf("matched %+v, want the movie", m.Title)
	}
}

func TestShowGateDetail(t *testing.T) {
	rules := GoodEveryday()
	p := releasename.Parse("Copper.Valley.S04.1080p.WEB-DL.H.264-QUILL", releasename.Hints{})
	set := SlotSet{Match: MatchTitle(p, showLibrary), SlotIDs: []int64{1}, MinSeeders: ptr(1), RuntimeMinutes: 450,
		GateDetail: "not every episode of season 4 has aired"}
	v := Judge(ReleaseFacts{Name: p.Original, Parse: p, Size: 12 * gb, Protocol: "torrent", Seeders: ptr(9)}, set, rules)
	if v.Outcome != "refused" || !slices.Contains(codes(v, "fail"), "hard.gate") {
		t.Fatalf("S28 %s\n%s", v.Outcome, dump(v))
	}
}

func TestRankEpisodes(t *testing.T) {
	rules := GoodEveryday()
	mk := func(name string, slots ...int64) (ReleaseFacts, Verdict) {
		p := releasename.Parse(name, releasename.Hints{})
		set := SlotSet{Match: MatchTitle(p, showLibrary), SlotIDs: slots, Out: true, RuntimeMinutes: 45 * len(slots)}
		rel := ReleaseFacts{Name: name, Parse: p, Size: int64(len(slots)) * 1200 * mb, Protocol: "torrent", PublishedAt: time.Now().Add(-time.Hour)}
		return rel, Judge(rel, set, rules)
	}
	r1, v1 := mk("Copper.Valley.S01E01.1080p.WEB-DL.H.264-QUILL", 1)
	r2, v2 := mk("Copper.Valley.S01.1080p.WEB-DL.H.264-QUILL", 1, 2, 3)
	rels, vs := []ReleaseFacts{r1, r2}, []Verdict{v1, v2}
	Rank(rels, vs, rules)
	if vs[1].Rank != 1 {
		t.Fatalf("same quality: the season pack should win:\n%s\n%s", dump(vs[0]), dump(vs[1]))
	}
	// A better single episode beats a pack on quality...
	r3, v3 := mk("Copper.Valley.S01E01.1080p.WEB-DL.H.265-QUILL", 1)
	rels, vs = []ReleaseFacts{r3, r2}, []Verdict{v3, v2}
	Rank(rels, vs, rules)
	if vs[0].Rank != 1 {
		t.Fatalf("quality first: %v %v", vs[0].Rank, vs[1].Rank)
	}
	// ...unless packs come first (season_pack_prefer = always).
	_, v2 = mk("Copper.Valley.S01.1080p.WEB-DL.H.264-QUILL", 1, 2, 3)
	_, v3 = mk("Copper.Valley.S01E01.1080p.WEB-DL.H.265-QUILL", 1)
	vs = []Verdict{v3, v2}
	RankPacksFirst(rels, vs, rules)
	if vs[1].Rank != 1 {
		t.Fatalf("packs first: %v %v", vs[0].Rank, vs[1].Rank)
	}
}

func TestGainsPerEpisode(t *testing.T) {
	rules := GoodEveryday()
	p := releasename.Parse("Copper.Valley.S01.1080p.WEB-DL.H.264-QUILL", releasename.Hints{})
	rel := ReleaseFacts{Name: p.Original, Parse: p}
	if !Gains(rel, rules, nil) {
		t.Fatal("a missing episode always gains")
	}
	low := CurrentOf("Copper.Valley.S01E01.720p.HDTV.x264-OXBOW", 0, time.Now(), rules)
	if !Gains(rel, rules, &low) {
		t.Fatal("1080p WEB over 720p TV gains")
	}
	same := CurrentOf("Copper.Valley.S01E01.1080p.WEB-DL.H.264-QUILL", 0, time.Now(), rules)
	if Gains(rel, rules, &same) {
		t.Fatal("the same copy does not gain")
	}
}

// A TMDB season that holds two parts: release names number the second part
// as season 2, and those names find the stored season 1 episodes.
func TestMatchEpisodesReleaseNumbers(t *testing.T) {
	var eps []EpisodeRef
	for e := 1; e <= 15; e++ {
		eps = append(eps, EpisodeRef{ItemID: int64(e), Season: 0, Episode: e})
	}
	for e := 1; e <= 25; e++ {
		r := EpisodeRef{ItemID: int64(100 + e), Season: 1, Episode: e, Absolute: e}
		if e > 12 {
			r.AltSeason, r.AltEpisode = 2, e-12
		}
		eps = append(eps, r)
	}
	labels := func(rs []EpisodeRef) []string {
		var out []string
		for _, r := range rs {
			out = append(out, r.Label())
		}
		return out
	}
	run := func(from, to int) []string {
		var out []string
		for e := from; e <= to; e++ {
			out = append(out, EpisodeRef{Season: 1, Episode: e}.Label())
		}
		return out
	}
	for _, c := range []struct {
		name string
		want []string
		why  string
	}{
		{"Mashle S02E05 1080p WEB-DL", []string{"S01E17"}, ""},
		{"[SubsPlease] Mashle S2 - 05 (1080p)", []string{"S01E17"}, ""},
		{"[Erai-raws] Mashle 2nd Season - 05 [1080p]", []string{"S01E17"}, ""},
		{"Mashle S01E05 1080p WEB-DL", []string{"S01E05"}, ""},
		{"Mashle S01E17 1080p WEB-DL", []string{"S01E17"}, ""},
		{"Mashle S01 1080p WEB-DL", run(1, 12), ""},
		{"Mashle S02 1080p WEB-DL", run(13, 25), ""},
		{"Mashle S01-S02 1080p WEB-DL", run(1, 25), ""},
		{"[SubsPlease] Mashle - 17 (1080p)", []string{"S01E17"}, ""},
		{"Mashle S03E01 1080p WEB-DL", nil, "the show has no episode S03E01"},
	} {
		got, why := MatchEpisodes(releasename.Parse(c.name, releasename.Hints{Show: true}), "absolute", eps)
		if !slices.Equal(labels(got), c.want) || why != c.why {
			t.Errorf("%s: %v %q, want %v %q", c.name, labels(got), why, c.want, c.why)
		}
	}
}
