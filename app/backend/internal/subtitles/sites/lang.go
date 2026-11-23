package sites

import (
	"slices"
	"strings"
)

// Machine-made kinds a site can flag on a subtitle.
const (
	AITranslated      = "ai_translated"
	MachineTranslated = "machine_translated"
)

// variantCodes are the kinds of a language kept apart from their base
// language (regions and scripts), with the spellings sites use for them. A
// plain base code means any kind. Each site turns these into its own codes
// on the way out and back through the tables beside its adapter.
var variantCodes = func() map[string]string {
	m := map[string]string{}
	for code, spellings := range map[string][]string{
		"pt-br":   {"pt_br", "ptbr", "pob", "pb", "br_pt", "br-pt"},
		"pt-pt":   {"pt_pt", "ptpt"},
		"zh-hans": {"zh-cn", "zh_cn", "zh-sg", "chs", "zh-hans-cn"},
		"zh-hant": {"zh-tw", "zh_tw", "zh-hk", "zh-mo", "cht", "zh-hant-tw", "zh-hant-hk"},
		"zh-yue":  {"yue", "zh-ca", "yue-hk"},
		"es-419":  {"es_419", "es-la", "es-mx", "es_mx", "spl"},
		"fr-ca":   {"fr_ca", "frca"},
	} {
		m[code] = code
		for _, s := range spellings {
			m[s] = code
		}
	}
	return m
}()

// siteLang turns a site's language code into ours: a kept kind (pt-br,
// zh-hant, es-419 and so on) in any spelling, and the base language for every
// other region.
func siteLang(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	if c, ok := variantCodes[code]; ok {
		return c
	}
	return baseLang(code)
}

// plainLangs is what a site with one kind per language is asked for: each
// wish's base code, plus the kinds the site keeps apart (has). A wish for a
// kind the site cannot tell apart is left out, because its answers could
// never meet that wish.
func plainLangs(asked []string, has ...string) []string {
	var out []string
	for _, l := range asked {
		if l = siteLang(l); l == "" {
			continue
		}
		if strings.Contains(l, "-") && !slices.Contains(has, l) {
			continue
		}
		if !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	return out
}

// langWanted reports whether a subtitle in got answers one of the asked
// languages: the same code, or a regional file for a base language, as a
// pt-BR file answers a pt wish.
func langWanted(asked []string, got string) bool {
	if got == "" {
		return false
	}
	if slices.Contains(asked, got) {
		return true
	}
	base, _, regional := strings.Cut(got, "-")
	return regional && slices.Contains(asked, base)
}
