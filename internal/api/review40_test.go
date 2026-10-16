package api

import (
	"testing"

	"github.com/CoderLuii/holyarrs/internal/domain"
)

// M10: an empty reason never panics, and the first letter is capitalised as a whole rune.
func TestDownloadViewReasonWords(t *testing.T) {
	empty, accent := "", "échec du serveur"
	if v := downloadView(domain.Download{State: "failed", StateReason: &empty}); v.Label != "Failed" {
		t.Fatalf("view %+v", v)
	}
	if v := downloadView(domain.Download{State: "failed", StateReason: &accent}); v.Text != "Échec du serveur." {
		t.Fatalf("text %q", v.Text)
	}
}
