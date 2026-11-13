package mediafiles

import (
	"fmt"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// benchTraits makes n synced traits shaped like TRaSH and Profilarr formats
// (group tiers, codecs, services, HDR, editions, audio, unwanted, language),
// each with its own patterns, and rules that score them all.
func benchTraits(n int) PickRulesBody {
	ts := make([]OwnTrait, 0, n)
	rules := GoodEveryday()
	rules.Traits = nil
	for i := 1; i <= n; i++ {
		var specs []OwnSpec
		switch i % 8 {
		case 0:
			groups := ""
			for g := 0; g < 20; g++ {
				if g > 0 {
					groups += "|"
				}
				groups += fmt.Sprintf("GRP%dX%d", i, g)
			}
			specs = []OwnSpec{{Name: "groups", Kind: "group", Value: "^(" + groups + ")$"},
				{Name: "web", Kind: "source", Value: "web_dl,web_rip"}}
		case 1:
			specs = []OwnSpec{{Name: "x265", Kind: "name", Value: fmt.Sprintf(`[xh][ ._-]?265|\bHEVC(\b|\d)|\bX%dC\b`, i), Required: true},
				{Name: "Not 2160p", Kind: "resolution", Value: "2160", Negate: true, Required: true}}
		case 2:
			specs = []OwnSpec{{Name: "service", Kind: "name", Value: fmt.Sprintf(`\b(amzn|amazon|svc%d)\b(?=[ ._-]web[ ._-]?(dl|rip)?\b)`, i)},
				{Name: "web", Kind: "source", Value: "web_dl,web_rip"}}
		case 3:
			specs = []OwnSpec{{Name: "DV", Kind: "name", Value: fmt.Sprintf(`(?<!HDR10[ ._-])\b(DV|DoVi|DVHDR%d)\b`, i), Required: true},
				{Name: "not SDR", Kind: "name", Value: `\bSDR\b`, Negate: true, Required: true}}
		case 4:
			specs = []OwnSpec{{Name: "edition", Kind: "name", Value: fmt.Sprintf(`(?<=\b[12]\d{3}\b).*\b(Extended|Uncut|Uncensored|Ed%d)\b`, i)}}
		case 5:
			specs = []OwnSpec{{Name: "audio", Kind: "name", Value: fmt.Sprintf(`\b(DD|E-?AC-?3|DDP|AU%d)[ ._-]?(5[ ._]1|7[ ._]1)\b`, i)}}
		case 6:
			specs = []OwnSpec{{Name: "unwanted", Kind: "name", Value: fmt.Sprintf(`(?<!\bTRUE)\b(LQ|LowQuality|BR-?DISK|UW%d)\b`, i)},
				{Name: "x264", Kind: "name", Value: `[xh][ ._-]?264`, Negate: true}}
		case 7:
			specs = []OwnSpec{{Name: "not original", Kind: "language", Value: "!original", Required: true},
				{Name: "not multi", Kind: "name", Value: fmt.Sprintf(`\b(MULTi|DUAL%d)\b`, i), Negate: true}}
		}
		ts = append(ts, OwnTrait{Meta: database.Meta{ID: int64(i)}, Name: fmt.Sprintf("Format %d", i), Match: "name", Specs: specs})
		rules.Traits = append(rules.Traits, TraitPoints{Trait: OwnTraitKey(int64(i)), Points: (i%21 - 10) * 100})
	}
	SetOwnTraits(ts)
	return rules
}

var benchNames = []string{
	"Movie.Title.2024.1080p.AMZN.WEB-DL.DDP5.1.H.264-GRP%dX3",
	"Movie.Title.2024.2160p.WEB-DL.DV.HDR10.DDP5.1.Atmos.H.265-FLUX%d",
	"Movie.Title.2024.Extended.1080p.BluRay.x264-SPARKS%d",
	"Movie.Title.2024.720p.WEBRip.x265.AAC2.0-RARBG%d",
	"Movie.Title.2024.MULTi.1080p.WEB.H264-FW%d",
	"Movie.Title.2024.1080p.BluRay.REMUX.AVC.DTS-HD.MA.7.1-FGT%d",
	"Movie Title 2024 1080p NF WEB-DL DDP5 1 HEVC-NTb%d",
	"Movie.Title.2024.German.DL.1080p.WEB.h265-LQ%d",
}

func benchSearch(n int) ([]ReleaseFacts, []SlotSet) {
	ref := TitleRef{ID: 1, Name: "Movie Title", Year: 2024, Runtime: 120, Kind: "movie", OriginalLanguage: "en"}
	now := time.Now().Add(-6 * time.Hour)
	rels := make([]ReleaseFacts, n)
	sets := make([]SlotSet, n)
	for i := range rels {
		name := fmt.Sprintf(benchNames[i%len(benchNames)], i)
		p := releasename.Parse(name, releasename.Hints{Size: 6 << 30})
		rels[i] = ReleaseFacts{Name: name, Parse: p, Size: 6 << 30, Protocol: "torrent", Seeders: ptr(40), PublishedAt: now}
		m := MatchTitle(p, []TitleRef{ref})
		sets[i] = SlotSet{Match: m, SlotIDs: []int64{1}, RuntimeMinutes: 120, Out: true, MinSeeders: ptr(1)}
	}
	return rels, sets
}

// BenchmarkJudgeTraits judges one Release, and a 200-Release search (judge
// every one, then rank), against rules with 64 and with 256 synced traits.
// A file already in the Library is set, so its points are counted too.
func BenchmarkJudgeTraits(b *testing.B) {
	for _, n := range []int{64, 113, 256} {
		rules := benchTraits(n)
		cur := CurrentOf("Movie.Title.2024.720p.WEB-DL.x264-OLD", 3<<30, time.Now().Add(-48*time.Hour), rules)
		rels, sets := benchSearch(200)
		for i := range sets {
			sets[i].Current = &cur
		}
		b.Run(fmt.Sprintf("one/traits=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = Judge(rels[0], sets[0], rules)
			}
		})
		b.Run(fmt.Sprintf("search200/traits=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			vs := make([]Verdict, len(rels))
			for b.Loop() {
				for i := range rels {
					vs[i] = Judge(rels[i], sets[i], rules)
				}
				Rank(rels, vs, rules)
			}
		})
	}
	SetOwnTraits(nil)
}
