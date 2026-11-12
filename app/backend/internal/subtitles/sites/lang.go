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

// regionCodes are the regional languages kept apart from their base
// language, with the spellings sites use for them.
var regionCodes = map[string]string{"pt-br": "pt-br", "pt_br": "pt-br", "ptbr": "pt-br", "pob": "pt-br", "pb": "pt-br", "br_pt": "pt-br", "br-pt": "pt-br"}

// siteLang turns a site's language code into ours: pt-br for Brazilian
// Portuguese in any spelling, and the base language for every other region.
func siteLang(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	if c, ok := regionCodes[code]; ok {
		return c
	}
	return baseLang(code)
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
