package mediafiles

import (
	"testing"
	"time"
)

func frRules(versions ...string) PickRulesBody {
	r := GoodEveryday()
	r.Language = LanguageRule{Wanted: []string{"fr"}, OriginalOK: true, FrenchVersions: versions}
	return r
}

func TestFrenchVersionPick544(t *testing.T) {
	const k = "The.Lantern.Keepers.2021."
	vfq := k + "MULTi.VFQ.1080p.WEB-DL.DDP5.1.H.265-QUILL"
	vff := k + "MULTi.TRUEFRENCH.1080p.WEB-DL.DDP5.1.H.265-QUILL"
	vf2 := k + "MULTi.VF2.1080p.WEB-DL.DDP5.1.H.265-QUILL"
	multi := k + "MULTi.1080p.WEB-DL.DDP5.1.H.265-QUILL"
	multi720 := k + "MULTi.720p.WEB-DL.DDP5.1.H.265-QUILL"
	vfq720 := k + "MULTi.VFQ.720p.WEB-DL.DDP5.1.H.265-QUILL"
	old := time.Now().Add(-30 * 24 * time.Hour)
	cur := func(name string, r PickRulesBody, info map[string]any) *Current {
		c := CurrentOf(JudgeName(name, info), 4<<30, old, r)
		return &c
	}
	for _, c := range []struct {
		name, rel string
		rules     PickRulesBody
		cur       *Current
		outcome   string
		code      string
	}{
		{"wanted version taken", vfq, frRules("VFQ"), nil, "chosen", "hard.french_version"},
		{"other known version refused", vff, frRules("VFQ"), nil, "refused", "hard.french_version"},
		{"no version allowed with no file", multi, frRules("VFQ"), nil, "chosen", ""},
		{"VF2 counts for VFQ", vf2, frRules("VFQ"), nil, "chosen", "hard.french_version"},
		{"no list: any version", vff, frRules(), nil, "chosen", ""},
		{"French not wanted: list ignored", vff, func() PickRulesBody { r := frRules("VFQ"); r.Language.Wanted = []string{"de"}; return r }(), nil, "chosen", ""},
		{"issue: MULTi 1080p does not replace VFQ 720p", multi, frRules("VFQ"), cur(vfq720, frRules("VFQ"), nil), "refused", "better.french_version"},
		{"VFQ 1080p replaces VFQ 720p", vfq, frRules("VFQ"), cur(vfq720, frRules("VFQ"), nil), "chosen", ""},
		{"VFQ replaces a MULTi with no version", vfq, frRules("VFQ"), cur(multi720, frRules("VFQ"), nil), "chosen", "better.french_version"},
		{"VFQ read after download counts", multi, frRules("VFQ"), cur(multi720, frRules("VFQ"), map[string]any{"french_version": "VFQ"}), "refused", "better.french_version"},
		{"no list: MULTi upgrade as before", multi, frRules(), cur(vfq720, frRules(), nil), "chosen", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			v := judgeDub(c.rel, c.rules, old, false, c.cur)
			if v.Outcome != c.outcome || (c.code != "" && !hasCode(v, c.code)) {
				t.Fatalf("outcome %s, want %s (%s): %+v", v.Outcome, c.outcome, c.code, v.Reasons)
			}
		})
	}
}

func TestFrenchVersionsCheck544(t *testing.T) {
	for _, c := range []struct {
		v  []string
		ok bool
	}{{nil, true}, {[]string{"VFF", "VFQ", "VFI", "VF2"}, true}, {[]string{"vfq"}, false}, {[]string{"VFQ", "VFQ"}, false}, {[]string{"FR"}, false}} {
		b := frRules(c.v...)
		if err := CheckPickRules("French", []string{"movie"}, b); (err == nil) != c.ok {
			t.Errorf("%v: err %v", c.v, err)
		}
	}
}
