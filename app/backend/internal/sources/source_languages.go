package sources

import (
	"slices"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"golang.org/x/text/language"
)

// checkLanguages is CleanLanguages for a list a person types: three-letter
// codes (ukr, ita) become the two-letter ones names use, and a code no
// release name can show is refused instead of dropped without a word.
func checkLanguages(in []string) ([]string, error) {
	out := []string{}
	for _, c := range in {
		c = strings.ToLower(strings.TrimSpace(c))
		if c == "" {
			continue
		}
		if !releasename.ReadsLanguage(c) {
			tag, err := language.Parse(c)
			base, conf := tag.Base()
			if err != nil || len(c) != 3 || conf == language.No || !releasename.ReadsLanguage(base.String()) {
				return nil, ErrLanguages
			}
			c = base.String()
		}
		if !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	return out, nil
}

// CleanLanguages keeps the language codes Release names can show, lower
// case and once each, for a Source's MULTi list. Never nil.
func CleanLanguages(in []string) []string {
	out := []string{}
	for _, c := range in {
		c = strings.ToLower(strings.TrimSpace(c))
		if c != "" && releasename.ReadsLanguage(c) && !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	return out
}
