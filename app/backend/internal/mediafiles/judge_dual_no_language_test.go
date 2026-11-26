package mediafiles

import (
	"strings"
	"testing"
	"time"
)

// TestDualNoLanguageNotOriginal (#797): DUAL or MULTi with no language word
// says the release has more than one audio language, so it is not read as
// the original language alone; it gets a warning instead of being turned away.
func TestDualNoLanguageNotOriginal(t *testing.T) {
	r := dubRules(0)
	r.Language.OriginalOK = false
	now := time.Now().Add(-48 * time.Hour)
	for _, name := range []string{
		"The.Lantern.Keepers.2021.DUAL.1080p.WEB-DL.DDP5.1.H.265-QUILL",
		"The.Lantern.Keepers.2021.MULTi.1080p.WEB-DL.DDP5.1.H.265-QUILL",
	} {
		v := judgeDub(name, r, now, true, nil)
		if v.Outcome == "refused" {
			t.Fatalf("%s turned away as the original language: %+v", name, v.Reasons)
		}
		found := false
		for _, x := range v.Reasons {
			if x.Rule == "language" && x.Effect == "warn" && strings.Contains(x.Detail, "checked at import") {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s: no import-check warning: %+v", name, v.Reasons)
		}
	}
}
