package mediafiles

import (
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

func TestPerMinuteSizeLimitArithmeticSaturates(t *testing.T) {
	tests := []struct {
		name    string
		release string
		hints   releasename.Hints
		title   TitleRef
		rules   PickRulesBody
		step    string
	}{
		{name: "video", release: "Example.Movie.2025.1080p.WEB-DL.H265-TEST", title: TitleRef{ID: 1, Name: "Example Movie", Year: 2025}, rules: GoodEveryday(), step: "1080p_web"},
		{name: "music", release: musicCase(t, "U01"), hints: releasename.Hints{Music: true}, title: TitleRef{ID: 2, Name: "Velvet Cartography", Kind: "artist"}, rules: LosslessFirst(), step: "lossless"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.rules.SizeLimits = map[string]SizeLimit{tc.step: {Min: 1 << 50, Max: 1 << 50, PerMinute: true}}
			p := releasename.Parse(tc.release, tc.hints)
			v := Judge(ReleaseFacts{Name: tc.release, Parse: p, Size: 1 << 30}, SlotSet{
				Match: Match{Title: &tc.title, Conf: releasename.Sure, Detail: "test match"}, SlotIDs: []int64{1}, RuntimeMinutes: 9000, Out: true,
			}, tc.rules)
			if !hasFail(v, "hard", "size") {
				t.Fatalf("overflowed lower bound did not fail size: %+v", v.Reasons)
			}
			if want := int64(math.MaxInt64) - 1<<30; v.sizeOff != want {
				t.Fatalf("size distance = %d, want saturated %d", v.sizeOff, want)
			}
		})
	}
}

// The fixed library from release-names-testset.md (movies only).
var library = []TitleRef{
	{ID: 1, Name: "The Lantern Keepers", Year: 2021, OriginalLanguage: "en", Runtime: 118},
	{ID: 2, Name: "Harbor of Glass", Year: 2018, OriginalLanguage: "en"},
	{ID: 3, Name: "Quiet Engines", Year: 2023, OriginalLanguage: "en"},
	{ID: 4, Name: "Moss and Iron", Year: 2016, OriginalLanguage: "en"},
	{ID: 5, Name: "The Paper Comet", Year: 2009, OriginalLanguage: "en"},
	{ID: 6, Name: "Velvet Static", Year: 2024, OriginalLanguage: "en"},
	{ID: 7, Name: "Northbound Tide", Year: 1999, OriginalLanguage: "en"},
	{ID: 8, Name: "The Last Orchard", Year: 2020, OriginalLanguage: "en"},
	{ID: 9, Name: "Saltwater Choir", Year: 2022, OriginalLanguage: "en"},
	{ID: 10, Name: "1871", Year: 2023, OriginalLanguage: "en"},
	{ID: 11, Name: "300 Lanterns", Year: 2015, OriginalLanguage: "en"},
	{ID: 12, Name: "Night Shift", Year: 2012, OriginalLanguage: "en"},
	{ID: 13, Name: "Night Shift", Year: 2019, OriginalLanguage: "en"},
	{ID: 14, Name: "The Clock Garden", Year: 2017, OriginalLanguage: "fr", Aliases: []string{"Le Jardin des Horloges"}},
	{ID: 15, Name: "Ice Castle", Year: 2020, OriginalLanguage: "ja", Aliases: []string{"Kōri no Shiro", "氷の城", "Ледяной Замок"}},
}

type row struct {
	ID       string
	Name     string
	Size     int64 // 0: the test set's default for the resolution
	Files    []string
	Current  *Current
	Outcome  string   // chosen or refused
	Points   int      // checked when the ReleaseFacts passes
	Codes    []string // refusal codes that must be present
	Warnings []string // warning codes that must be present
	Title    int64    // matched Title id, 0 for none
}

// Rows: M01 to M22 and the movie rows of the tricky table.
// Where the test set's verdict column disagrees with the preset arithmetic,
// the preset wins and the row says why (see "Corrections from the Go table"
// in release-names-testset.md).
var rows = []row{
	{ID: "M01", Name: "The.Lantern.Keepers.2021.1080p.WEB-DL.DDP5.1.H.265-QUILL", Outcome: "chosen", Points: 20, Title: 1},
	{ID: "M02", Name: "The.Lantern.Keepers.2021.1080p.WEBRip.x264.AAC-TINCAN", Outcome: "chosen", Points: 0, Title: 1},
	{ID: "M03", Name: "Harbor.of.Glass.2018.Directors.Cut.1080p.BluRay.x264.DTS-MOSSFOX", Outcome: "chosen", Points: 0, Title: 2},
	{ID: "M04", Name: "Harbor.of.Glass.2018.EXTENDED.REMASTERED.720p.BluRay.x264-OXBOW", Outcome: "chosen", Points: 0, Title: 2},
	{ID: "M05", Name: "Quiet.Engines.2023.2160p.UHD.BluRay.REMUX.DV.HDR10.HEVC.TrueHD.7.1.Atmos-LARKSPUR", Outcome: "refused", Codes: []string{"step.not_allowed"}, Title: 3},
	{ID: "M06", Name: "Quiet.Engines.2023.2160p.WEB-DL.DDP5.1.HDR10Plus.H.265-QUILL", Outcome: "refused", Codes: []string{"step.not_allowed"}, Title: 3},
	{ID: "M07", Name: "Quiet.Engines.2023.1080p.BluRay.REMUX.AVC.DTS-HD.MA.5.1-LARKSPUR", Outcome: "refused", Codes: []string{"step.not_allowed"}, Title: 3},
	{ID: "M08", Name: "Moss.and.Iron.2016.3D.HSBS.1080p.BluRay.x264-PIXELORCHARD", Outcome: "refused", Codes: []string{"hard.never:3d"}, Title: 4},
	{ID: "M09", Name: "Moss.&.Iron.2016.1080p.AMZN.WEB-DL.DDP5.1.H.264-GRAVELY", Outcome: "chosen", Points: 10, Title: 4},
	{ID: "M10", Name: "The.Paper.Comet.2009.720p.HDTV.x264-OXBOW", Outcome: "chosen", Points: 0, Title: 5},
	{ID: "M11", Name: "The.Paper.Comet.2009.DVDRip.XviD-OLDREEL", Outcome: "refused", Codes: []string{"step.not_allowed"}, Title: 5},
	{ID: "M12", Name: "The.Paper.Comet.2009.COMPLETE.BLURAY-DISCWORKS", Outcome: "refused", Codes: []string{"hard.safety:disc_image"}, Title: 5},
	{ID: "M13", Name: "Velvet.Static.2024.HDCAM.x264-GRAVELY", Outcome: "refused", Codes: []string{"hard.never:cam", "step.not_allowed"}, Title: 6},
	// The test set lists only "telesync not allowed"; the preset's never list
	// also has telesync, so both reasons are reported (as M13 does for cam).
	{ID: "M14", Name: "Velvet.Static.2024.HDTS.720p.x264-TINCAN", Outcome: "refused", Codes: []string{"hard.never:telesync", "step.not_allowed"}, Title: 6},
	{ID: "M15", Name: "Velvet Static (2024) [1080p] [WEBRip] [5.1] [YTSLIKE]", Outcome: "chosen", Points: 0, Title: 6},
	{ID: "M16", Name: "Northbound.Tide.1999.REMASTERED.1080p.BluRay.10bit.x265.AAC-QUILL", Outcome: "chosen", Points: 10, Title: 7},
	{ID: "M17", Name: "Northbound.Tide.1999.IMAX.2160p.WEB-DL.DV.HDR.H.265-QUILL", Outcome: "refused", Codes: []string{"step.not_allowed"}, Title: 7},
	{ID: "M18", Name: "Northbound.Tide.1999.1080p.WEB-DL.HLG.H.265-QUILL", Outcome: "chosen", Points: 15, Title: 7},
	// "WEB" alone is web_dl at "likely"; points Traits need a sure field, so 0.
	{ID: "M19", Name: "The.Last.Orchard.2020.UNRATED.1080p.WEB.h264-MOSSFOX", Outcome: "chosen", Points: 0, Title: 8},
	{ID: "M20", Name: "The.Last.Orchard.2020.Theatrical.Cut.1080p.WEB-DL.AV1.Opus-PIXELORCHARD", Outcome: "chosen", Points: 5, Title: 8},
	{ID: "M21", Name: "Saltwater.Choir.2022.1080p.WEB-DL.DDP5.1.Atmos.H.264-QUILL.mkv", Outcome: "chosen", Points: 10, Title: 9},
	{ID: "M22", Name: "Saltwater.Choir.2022.720p.WEB-DL.x264-TINCAN", Outcome: "chosen", Points: 5, Title: 9},

	{ID: "T01", Name: "1871.2023.1080p.WEB-DL.H.264-QUILL", Outcome: "chosen", Points: 5, Title: 10},
	// "Needs a decision" is not a Verdict outcome: the match is a guess, so the
	// Release is refused with match.needs_hand_pick until a person picks.
	{ID: "T02", Name: "1871.1080p.WEB-DL.H.264-QUILL", Outcome: "refused", Codes: []string{"match.needs_hand_pick"}, Title: 10},
	{ID: "T03", Name: "300.Lanterns.2015.720p.BluRay.x264-OXBOW", Outcome: "chosen", Points: 0, Title: 11},
	{ID: "T04", Name: "Night.Shift.2012.1080p.WEB-DL-QUILL", Outcome: "chosen", Points: 5, Title: 12},
	{ID: "T05", Name: "Night.Shift.2020.1080p.WEB-DL-QUILL", Outcome: "chosen", Points: 5, Title: 13, Warnings: []string{"match.year_near"}},
	{ID: "T06", Name: "Night.Shift.2015.1080p.WEB-DL-QUILL", Outcome: "refused", Codes: []string{"match.none"}},
	{ID: "T07", Name: "Le.Jardin.des.Horloges.2017.FRENCH.1080p.BluRay.x264-LESQUAIS", Outcome: "chosen", Points: 0, Title: 14},
	{ID: "T08", Name: "The.Clock.Garden.2017.GERMAN.DL.1080p.BluRay.x264-NORDFILM", Outcome: "chosen", Points: 0, Title: 14},
	{ID: "T09", Name: "The.Clock.Garden.2017.iTALiAN.DUBBED.1080p.WEB-DL-TINCAN", Outcome: "refused", Codes: []string{"hard.language"}, Title: 14},
	{ID: "T10", Name: "Kōri.no.Shiro.2020.1080p.WEB-DL.H.264-Kumomaru", Outcome: "chosen", Points: 5, Title: 15},
	{ID: "T11", Name: "氷の城.2020.1080p.WEB-DL-Kumomaru", Outcome: "chosen", Points: 5, Title: 15},
	{ID: "T12", Name: "Ледяной.Замок.2020.1080p.WEB-DL.RUS-SNEGOVIK", Outcome: "refused", Codes: []string{"hard.language"}, Title: 15},
	{ID: "T13", Name: "The.Lantern.Keepers.2021.1080p.WEB-DL.H.264-QUILL.exe", Outcome: "refused", Codes: []string{"hard.safety:executable"}, Title: 1},
	{ID: "T14", Name: "The.Lantern.Keepers.2021.1080p.WEB-DL.H.264-QUILL", Files: []string{"setup.exe", "readme.txt"}, Outcome: "refused", Codes: []string{"hard.safety:executable"}, Title: 1},
	{ID: "T15", Name: "The.Lantern.Keepers.2021.1080p.WEB-DL.H.264-QUILL.mkv.lnk", Outcome: "refused", Codes: []string{"hard.safety:double_extension", "hard.safety:executable"}, Title: 1},
	{ID: "T16", Name: "The.Lantern.Keepers.2021.1080p.BluRay.x264-QUILL.PASSWORDED", Outcome: "refused", Codes: []string{"hard.safety:password_protected"}, Title: 1},
	// 650 MB for 118 minutes is above one fifth of the 0.5 GB/h minimum, so
	// it is not "fake by size"; it is under the size limit and the step is not allowed.
	{ID: "T17", Name: "The.Lantern.Keepers.2021.2160p.WEB-DL-QUILL", Size: 650 * mb, Outcome: "refused", Codes: []string{"step.not_allowed", "hard.size"}, Warnings: []string{"hard.size_mismatch"}, Title: 1},
	// 300 MB for 118 minutes: under the size limit, not under one fifth of it.
	{ID: "T18", Name: "The.Lantern.Keepers.2021.1080p.WEB-DL-QUILL", Size: 300 * mb, Outcome: "refused", Codes: []string{"hard.size"}, Warnings: []string{"hard.size_mismatch"}, Title: 1},
	{ID: "T19", Name: "The.Lantern.Keepers.2021.SAMPLE.1080p.WEB-DL-QUILL", Size: 60 * mb, Outcome: "refused", Codes: []string{"hard.safety:sample"}, Title: 1},
	{ID: "T20", Name: "The.Lantern.Keepers.2021.Featurettes.1080p.WEB-DL-QUILL", Outcome: "refused", Codes: []string{"hard.safety:extras_only"}, Title: 1},
	{ID: "T21", Name: "The.Lantern.Keepers.2021.720p.1080p.WEB-DL-TINCAN", Outcome: "chosen", Points: 5, Warnings: []string{"parse.resolution_guess"}, Title: 1},
	// The test set says "ok -45"; min_points is 0 in the preset, so -45 is refused.
	{ID: "T22", Name: "The.Lantern.Keepers.2021.1080p.WEB-DL.x264.UPSCALED-GRAVELY", Outcome: "refused", Codes: []string{"points.below_min"}, Title: 1},
	{ID: "T23", Name: "The.Lantern.Keepers.2021.PROPER.1080p.WEB-DL.H.265-QUILL", Current: &Current{Step: "1080p_web", Points: 15, Group: "QUILL", ImportedAt: time.Now().Add(-72 * time.Hour)}, Outcome: "chosen", Points: 15, Title: 1},
	// The test set says "fix older than 7 days"; the proper policy is take_any
	// with no time limit, so the fix is taken.
	{ID: "T24", Name: "The.Lantern.Keepers.2021.PROPER.1080p.WEB-DL.H.265-QUILL", Current: &Current{Step: "1080p_web", Points: 15, Group: "QUILL", ImportedAt: time.Now().Add(-20 * 24 * time.Hour)}, Outcome: "chosen", Points: 15, Title: 1},
	{ID: "T25", Name: "The.Lantern.Keepers.2021.720p.WEB-DL-QUILL", Current: &Current{Step: "1080p_web", Points: 5, Group: "QUILL"}, Outcome: "refused", Codes: []string{"better.lower_step"}, Title: 1},
	{ID: "T26", Name: "Lantern Keepers, The (2021) 1080p WEB-DL", Outcome: "chosen", Points: 5, Title: 1},
	{ID: "T28", Name: "www.somesite.example - The.Lantern.Keepers.2021.1080p.WEB-DL-QUILL", Outcome: "chosen", Points: 5, Title: 1},
}

// defaultSize follows the test set: movie 1080p 4 GB, 720p 2 GB, 2160p 15 GB.
func defaultSize(res int) int64 {
	switch res {
	case 2160:
		return 15 * gb
	case 1080:
		return 4 * gb
	}
	return 2 * gb
}

func release(r row) ReleaseFacts {
	p := releasename.Parse(r.Name, releasename.Hints{Size: r.Size, FileList: r.Files})
	size := r.Size
	if size == 0 {
		size = defaultSize(p.Resolution.Value)
	}
	return ReleaseFacts{Name: r.Name, Parse: p, Size: size, Protocol: "torrent", Seeders: ptr(85),
		PublishedAt: time.Now().Add(-6 * time.Hour), Files: r.Files}
}

func ptr[T any](v T) *T { return &v }

func setFor(t *testing.T, rel ReleaseFacts, cur *Current) SlotSet {
	t.Helper()
	m := MatchTitle(rel.Parse, library)
	runtime := 120
	if m.Title != nil && m.Title.Runtime > 0 {
		runtime = m.Title.Runtime
	}
	return SlotSet{Match: m, SlotIDs: []int64{1}, RuntimeMinutes: runtime, Out: true, MinSeeders: ptr(1), Current: cur}
}

func codes(v Verdict, effect string) []string {
	var out []string
	for _, r := range v.Reasons {
		if r.Effect == effect {
			out = append(out, r.Code())
		}
	}
	return out
}

func TestMovieVerdicts(t *testing.T) {
	rules := GoodEveryday()
	for _, r := range rows {
		t.Run(r.ID, func(t *testing.T) {
			rel := release(r)
			set := setFor(t, rel, r.Current)
			if r.Title == 0 && set.Match.Title != nil {
				t.Fatalf("matched title %d, want none", set.Match.Title.ID)
			}
			if r.Title != 0 && (set.Match.Title == nil || set.Match.Title.ID != r.Title) {
				t.Fatalf("matched %+v, want title %d", set.Match.Title, r.Title)
			}
			v := Judge(rel, set, rules)
			if len(v.Reasons) == 0 {
				t.Fatal("verdict has no reasons")
			}
			for _, rs := range v.Reasons {
				if rs.Detail == "" {
					t.Errorf("reason %s has no sentence", rs.Code())
				}
			}
			if v.Outcome != r.Outcome {
				t.Fatalf("outcome %s, want %s; reasons:\n%s", v.Outcome, r.Outcome, dump(v))
			}
			if v.Passed != (r.Outcome != "refused") {
				t.Errorf("passed %v for outcome %s", v.Passed, v.Outcome)
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
			if r.Outcome == "refused" && len(fails) == 0 {
				t.Error("refused with no failing reason")
			}
			warns := codes(v, "warn")
			for _, c := range r.Warnings {
				if !slices.Contains(warns, c) {
					t.Errorf("missing warning %s; got %v", c, warns)
				}
			}
		})
	}
}

func dump(v Verdict) string {
	var b strings.Builder
	for _, r := range v.Reasons {
		b.WriteString("  " + r.Code() + " " + r.Effect + ": " + r.Detail + "\n")
	}
	return b.String()
}

func TestAllHardRefusalsAtOnce(t *testing.T) {
	// Worked example C: hardcoded subs and cam are both reported.
	rel := release(row{Name: "The.Lantern.Keepers.2021.1080p.HDCAM.HC.x264-GRAVELY", Size: 1400 * mb})
	v := Judge(rel, setFor(t, rel, nil), GoodEveryday())
	fails := codes(v, "fail")
	for _, c := range []string{"hard.never:hardcoded_subs", "step.not_allowed"} {
		if !slices.Contains(fails, c) {
			t.Errorf("missing %s in %v", c, fails)
		}
	}
	for _, r := range v.Reasons {
		if r.Step == "points" || r.Step == "wait" {
			t.Errorf("step %s ran after a refusal", r.Step)
		}
	}
}

func TestHardChecksRunInSpecOrder(t *testing.T) {
	rel := release(row{Name: "The.Lantern.Keepers.2021.1080p.WEB-DL.H.264-QUILL.exe"})
	v := Judge(rel, setFor(t, rel, nil), GoodEveryday())
	var order []string
	for _, r := range v.Reasons {
		if r.Step != "hard" {
			continue
		}
		name, _, _ := strings.Cut(r.Rule, ":")
		if len(order) == 0 || order[len(order)-1] != name {
			order = append(order, name)
		}
	}
	rank := map[string]int{"never": 0, "must_have": 1, "safety": 2, "size": 3, "language": 4, "seeders": 5, "gate": 6, "turned_down": 7}
	last := -1
	for _, n := range order {
		if rank[n] < last {
			t.Fatalf("hard checks out of order %v:\n%s", order, dump(v))
		}
		last = rank[n]
	}
	if !strings.Contains(strings.Join(order, ","), "safety") {
		t.Fatalf("safety did not run: %v", order)
	}
}

func TestRankWorkedExamples(t *testing.T) {
	// Worked examples A and B: same step, B loses on points.
	rules := GoodEveryday()
	var vs []Verdict
	var rels []ReleaseFacts
	for _, name := range []string{
		"The.Lantern.Keepers.2021.1080p.WEBRip.x264.AAC-TINCAN",
		"The.Lantern.Keepers.2021.1080p.WEB-DL.DDP5.1.H.265-QUILL",
		"The.Lantern.Keepers.2021.1080p.HDCAM.HC.x264-GRAVELY",
	} {
		rel := release(row{Name: name})
		rels = append(rels, rel)
		vs = append(vs, Judge(rel, setFor(t, rel, nil), rules))
	}
	Rank(rels, vs, rules)
	if vs[1].Outcome != "chosen" || vs[1].Rank != 1 {
		t.Fatalf("QUILL should be chosen: %s rank %d", vs[1].Outcome, vs[1].Rank)
	}
	if vs[0].Outcome != "passed_over" || vs[0].Rank != 2 {
		t.Fatalf("TINCAN should be passed over: %s rank %d", vs[0].Outcome, vs[0].Rank)
	}
	last := vs[0].Reasons[len(vs[0].Reasons)-1]
	want := "lost to The.Lantern.Keepers.2021.1080p.WEB-DL.DDP5.1.H.265-QUILL on points (0 vs 20)"
	if last.Effect != "rank" || last.Detail != want {
		t.Fatalf("loser reason %q, want %q", last.Detail, want)
	}
	if vs[2].Outcome != "refused" || vs[2].Rank != 0 {
		t.Fatalf("refused release should stay refused and unranked: %+v", vs[2].Outcome)
	}
}

func TestRankQualityFirst(t *testing.T) {
	rules := GoodEveryday()
	a := release(row{Name: "Saltwater.Choir.2022.720p.WEB-DL.H.265-QUILL"})  // 720p_web, 15 points
	b := release(row{Name: "Saltwater.Choir.2022.1080p.WEBRip.x264-TINCAN"}) // 1080p_web, 0 points
	vs := []Verdict{Judge(a, setFor(t, a, nil), rules), Judge(b, setFor(t, b, nil), rules)}
	Rank([]ReleaseFacts{a, b}, vs, rules)
	if vs[1].Outcome != "chosen" {
		t.Fatalf("quality_then_points: the higher step wins\n%s", dump(vs[0]))
	}
	rules.Order = "points_then_quality"
	vs = []Verdict{Judge(a, setFor(t, a, nil), rules), Judge(b, setFor(t, b, nil), rules)}
	Rank([]ReleaseFacts{a, b}, vs, rules)
	if vs[0].Outcome != "chosen" {
		t.Fatal("points_then_quality: more points wins")
	}
}

func TestProtocolWaitHolds(t *testing.T) {
	rules := GoodEveryday()
	rules.Protocol.WaitMinutesTorrent = 60
	rel := release(row{Name: "Saltwater.Choir.2022.720p.WEB-DL.x264-TINCAN"})
	rel.PublishedAt = time.Now().Add(-10 * time.Minute)
	v := Judge(rel, setFor(t, rel, nil), rules)
	if v.Outcome != "held" || v.HoldUntil == nil {
		t.Fatalf("young release should be held: %s\n%s", v.Outcome, dump(v))
	}
	set := setFor(t, rel, nil)
	set.HandSearch = true
	if v := Judge(rel, set, rules); v.Outcome != "chosen" {
		t.Fatalf("a hand search skips the wait: %s", v.Outcome)
	}
}

func TestGateSeedersAndTurnedDown(t *testing.T) {
	rules := GoodEveryday()
	rel := release(row{Name: "Saltwater.Choir.2022.720p.WEB-DL.x264-TINCAN"})
	rel.Seeders = ptr(0)
	set := setFor(t, rel, nil)
	set.Out = false
	set.TurnedDown = []string{rel.Name}
	fails := codes(Judge(rel, set, rules), "fail")
	for _, c := range []string{"hard.seeders", "hard.gate", "hard.turned_down"} {
		if !slices.Contains(fails, c) {
			t.Errorf("missing %s in %v", c, fails)
		}
	}
}
