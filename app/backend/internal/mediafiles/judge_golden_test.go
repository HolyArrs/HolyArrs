package mediafiles

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

var updateGolden = flag.Bool("update-golden", false, "rewrite testdata/judge_golden.json")

// GoldenNames is a fixed set of Release names judged by the golden tests.
func GoldenNames() []string {
	titles := []string{"Movie.Title.2021", "Show.Name.S02E05", "Show.Name.S03", "Another.Film.1999"}
	parts := []string{
		"2160p.WEB-DL.DDP5.1.HDR.DV.H.265-GOODA", "1080p.WEB-DL.DDP5.1.H.264-GOODB",
		"1080p.WEBRip.x265-RARBG", "720p.HDTV.x264-LOL", "1080p.BluRay.x264-SPARKS",
		"2160p.BluRay.REMUX.HEVC.TrueHD.Atmos-FGT", "1080p.BluRay.REMUX.AVC.DTS-HD.MA-EPSiLON",
		"480p.DVDRip.XviD-DEMAND", "CAM.x264-BADGRP", "HDTS.720p-BADGRP",
		"1080p.WEB.h264-NTb", "2160p.UPSCALED.WEB-DL.H265-XX", "1080p.WEB-DL.German.DL.x264-WvF",
		"1080p.AMZN.WEB-DL.DDP2.0.H.264-NTb", "720p.WEB-DL.AAC2.0.H.264.Dubbed-ABC",
		"1080p.HDTV.MULTI.x264-FRA", "2160p.WEB-DL.HDR10.HEVC-GOODA", "1080p.BluRay.10bit.x265.HEVC-Tigole",
		"DVD9.COMPLETE-DiSC", "1080p.Directors.Cut.BluRay.x264-AMIABLE",
		"1080p.PROPER.WEB-DL.H.264-GOODA", "720p.WEB-DL.REPACK.H264-KiNGS",
		"1080p.iT.WEB-DL.DD5.1.H.264-Japanese", "1080p.WEB-DL.x264.HC-KORSUB",
		"2160p.WEB-DL.DV.HDR10Plus.H.265-FLUX", "1080p.NF.WEBRip.DDP5.1.x264-NTG",
		"1080p.REMUX.MPEG-2.DD5.1-BLURANiUM", "576p.PAL.DVDRip-SD", "1080p.3D.BluRay.x264-3DGRP",
		"1080p.WEB-DL.Hindi.x264-HIN", "1080p.WEB-DL.exe-SCAM", "720p.Extended.BluRay.x264-REWARD",
		"1080p.HMAX.WEB-DL.Atmos.H.264-CMRG", "2160p.ATVP.WEB-DL.DDP5.1.HDR.H.265-GOODB",
		"1080p.BDRip.x264-AMIABLE", "1080p.TELESYNC.x264-BADGRP", "1080p.Remastered.BluRay.x265-PSA",
		"1080p.WEB-DL.Spanish.x264-ESP", "1080p.HDTV.h264-Arabic", "2160p.UHD.BluRay.x265-TERMINAL",
		"1080p.WEB-DL.H.264-SAMPLE.sample", "1080p.IMAX.WEB-DL.DDP5.1.H.264-GOODA", "720p.WEB.x265-MeGusta",
		"1080p.Unrated.WEB-DL.H264-GOODA", "SDTV.XviD-OLD", "1080p.WEB-DL.Opus.AV1-AV1GRP",
		"1080p.WEB-DL.FLAC.H.264-Lossless", "1080p.WEB-DL.Hybrid.H.264-HYB", "2160p.HDR.REMUX.DV-FraMeSToR",
		"1080p.WEB-DL.DUAL.x264-DUO",
	}
	var out []string
	for i, p := range parts {
		out = append(out, titles[i%len(titles)]+"."+p, titles[(i+1)%len(titles)]+"."+p, titles[(i+2)%len(titles)]+"."+p, titles[(i+3)%len(titles)]+"."+p)
	}
	return out
}

// goldenTraits are own traits of both forms (typed terms and synced tests)
// that the golden rules give points to.
func goldenTraits() []OwnTrait {
	return []OwnTrait{
		{Meta: database.Meta{ID: 901}, Name: "Good groups", Match: "group", Terms: []string{"GOODA", "/^GOODB$/"}},
		{Meta: database.Meta{ID: 902}, Name: "Atmos", Match: "name", Terms: []string{"atmos"}},
		{Meta: database.Meta{ID: 903}, Name: "Big", Match: "size", Terms: []string{"10-"}},
		{Meta: database.Meta{ID: 904}, Name: "HEVC not 4K", Match: "name", Source: "trash", SourceID: "sonarr:x", Specs: []OwnSpec{
			{Name: "x265", Kind: "name", Value: `[xh][ ._-]?265|\bHEVC\b`, Required: true},
			{Name: "Not 2160p", Kind: "resolution", Value: "2160", Negate: true, Required: true},
		}},
		{Meta: database.Meta{ID: 905}, Name: "Web", Match: "name", Source: "trash", SourceID: "sonarr:y", Specs: []OwnSpec{
			{Name: "WEB-DL", Kind: "source", Value: "web_dl"}, {Name: "WEBRip", Kind: "source", Value: "web_rip"},
		}},
		{Meta: database.Meta{ID: 906}, Name: "DV no HDR10", Match: "name", Source: "trash", SourceID: "radarr:z", Specs: []OwnSpec{
			{Name: "DV", Kind: "name", Value: `(?<!HDR10[ ._-])\bDV\b`},
			{Name: "Size", Kind: "size", Value: "1-80"},
		}},
	}
}

type goldenRow struct {
	Preset  string   `json:"preset"`
	Name    string   `json:"name"`
	Outcome string   `json:"outcome"`
	Passed  bool     `json:"passed"`
	Points  int      `json:"points"`
	Good    bool     `json:"good_enough"`
	Reasons []string `json:"reasons"`
}

// TestJudgeGolden pins what Judge says for every ready-made Pick rules
// and own traits: new trait test kinds must never change a pick for
// someone who has none of them.
func TestJudgeGolden(t *testing.T) {
	SetOwnTraits(goldenTraits())
	t.Cleanup(func() { SetOwnTraits(nil) })
	withOwn := GoodEveryday()
	for _, id := range []int64{901, 902, 903, 904, 905, 906} {
		withOwn.Traits = append(withOwn.Traits, TraitPoints{Trait: OwnTraitKey(id), Points: int(id) - 900})
	}
	sets := []struct {
		name string
		body PickRulesBody
	}{{"standard", GoodEveryday()}, {"lite", SaveSpace()}, {"best", Best()}, {"4k", FourK()}, {"lossless", LosslessFirst()}, {"own", withOwn}}
	var rows []goldenRow
	for _, s := range sets {
		for i, n := range GoldenNames() {
			p := releasename.Parse(n, releasename.Hints{Music: IsMusicRules(s.body)})
			rel := ReleaseFacts{Name: n, Parse: p, Size: int64(i%20+1) << 30, Protocol: "torrent", Seeders: ptr(5)}
			kind := "movie"
			if p.IsShow() {
				kind = "show"
			}
			m := Match{Title: &TitleRef{ID: 1, Name: p.Title.Value, Year: p.Year.Value, Runtime: 60, Kind: kind, OriginalLanguage: "en"}, Conf: releasename.Sure}
			v := Judge(rel, SlotSet{Match: m, SlotIDs: []int64{1}, RuntimeMinutes: 60, Out: true, MinSeeders: ptr(1)}, s.body)
			r := goldenRow{Preset: s.name, Name: n, Outcome: v.Outcome, Passed: v.Passed, Points: v.Points, Good: v.GoodEnough}
			for _, rs := range v.Reasons {
				r.Reasons = append(r.Reasons, rs.Step+"|"+rs.Rule+"|"+rs.Effect+"|"+rs.Detail)
			}
			rows = append(rows, r)
		}
	}
	got, _ := json.MarshalIndent(rows, "", " ")
	got = append(got, '\n')
	path := filepath.Join("testdata", "judge_golden.json")
	if *updateGolden {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n")), got) {
		t.Fatal("Judge output changed for rules without the new trait tests; see testdata/judge_golden.json")
	}
}
