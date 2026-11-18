package mediafiles

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

func TestNameShapesReported(t *testing.T) {
	lib := []TitleRef{
		{ID: 1, Name: "Schitt's Creek", Kind: "show"},
		{ID: 2, Name: "Grey's Anatomy", Kind: "show"},
		{ID: 3, Name: "RuPaul's Drag Race", Kind: "show"},
		{ID: 4, Name: "The Handmaid's Tale", Kind: "show"},
		{ID: 5, Name: "Workin' Moms", Kind: "show"},
		{ID: 6, Name: "Bob's Burgers", Kind: "show"},
		{ID: 7, Name: "Schitt’s Creek 2", Kind: "show"},
	}
	cases := []struct {
		rel string
		id  int64
	}{
		{"Schitts Creek S04E02 Pregnancy Test 1080p AMZN WEB-DL DDP5 1 H 264-Kitsune", 1},
		{"Schitts Creek S04E02 Pregnancy Test 1080p DSNP WEB-DL DDP5 1 H 264-FLUX", 1},
		{"Schitts.Creek.S04E02.1080p.WEBRip.x264-TBS", 1},
		{"Schitts.Creek.S04E02.720p.WEBRip.x264-TBS", 1},
		{"Schitts.Creek.S04E02.720p.WEBRip.x264-TBS[ettv]", 1},
		{"Schitts.Creek.S04E02.WEBRip.x264-TBS", 1},
		{"Schitts.Creek.S04E02.WEBRip.x264-TBS[ettv]", 1},
		{"Schitts.Creek.S04E02.XviD-AFG", 1},
		{"Schitts.Creek.S04E02.480p.x264-mSD", 1},
		{"Schitts Creek S04 1080p AMZN WEB-DL DDP5 1 H 264-Kitsune", 1},
		{"Greys Anatomy S20E01 We've Only Just Begun 1080p AMZN WEB-DL DDP5 1 H 264-NTb", 2},
		{"Greys.Anatomy.S20E01.720p.HDTV.x264-SYNCOPY[eztv.re].mkv", 2},
		{"RuPauls.Drag.Race.S16E01.1080p.WEB.h264-EDITH", 3},
		{"RuPauls Drag Race S16E01 Rate A Queen 1080p AMZN WEB-DL DDP2 0 H 264-NTb", 3},
		{"The.Handmaids.Tale.S05E01.Morning.1080p.HULU.WEB-DL.DDP5.1.H.264-NTb", 4},
		{"Workin.Moms.S07E01.1080p.WEB.h264-BAE", 5},
		{"Workin Moms S07E01 720p WEBRip x264-BAE[eztv]", 5},
		{"Bobs.Burgers.S14E01.1080p.WEB.h264-BAE", 6},
		{"Bobs Burgers S14E01 Fight at the Not Okay Chore-ral 1080p DSNP WEB-DL DDP5 1 H 264-NTb", 6},
	}
	bad := 0
	for _, c := range cases {
		p := releasename.Parse(c.rel, releasename.Hints{Show: true})
		m := MatchTitle(p, lib)
		if m.Title == nil || m.Title.ID != c.id {
			bad++
			t.Errorf("REFUSED %q parsed=%q detail=%q", c.rel, p.Title.Value, m.Detail)
		}
	}
	t.Logf("reporter shapes: %d cases, %d refused", len(cases), bad)
}
