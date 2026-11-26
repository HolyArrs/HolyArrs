package mediafiles

import (
	"testing"
	"time"
)

// With only German wanted and the original language not allowed, a name
// with no language word is read as the original language and turned away,
// so it cannot beat a German release on points.
func TestNoLanguageOriginalNotWanted(t *testing.T) {
	bare := "The.Lantern.Keepers.2021.1080p.WEB-DL.DDP5.1.Atmos.H.265-QUILL"
	de := "The.Lantern.Keepers.2021.German.DL.1080p.WEB-DL.DDP5.1.H.265-QUILL"
	r := dubRules(0)
	r.Language.OriginalOK = false
	now := time.Now().Add(-48 * time.Hour)
	if v := judgeDub(bare, r, now, true, nil); v.Outcome != "refused" {
		t.Fatalf("untagged original-language release should be turned away, got %s: %+v", v.Outcome, v.Reasons)
	}
	if v := judgeDub(de, r, now, true, nil); v.Outcome == "refused" {
		t.Fatalf("German release turned away: %+v", v.Reasons)
	}
	r.Language.OriginalOK = true
	if v := judgeDub(bare, r, now, true, nil); v.Outcome == "refused" {
		t.Fatalf("untagged release turned away with the original language allowed: %+v", v.Reasons)
	}
}
