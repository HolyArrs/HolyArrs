package downloads

import (
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// M10: an empty reason never panics, and the first letter is capitalised as a whole rune.
func TestDownloadViewReasonWords(t *testing.T) {
	t.Parallel()
	empty, accent := "", "échec du serveur"
	if v := DownloadViewOf(domain.Download{State: "failed", StateReason: &empty}); v.Label != "Failed" {
		t.Fatalf("view %+v", v)
	}
	if v := DownloadViewOf(domain.Download{State: "failed", StateReason: &accent}); v.Text != "Échec du serveur." {
		t.Fatalf("text %q", v.Text)
	}
}
