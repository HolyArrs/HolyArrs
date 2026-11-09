package subtitles

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// Importing this package alone must give the library its subtitle rules.
func TestNamesRegisteredOnImport(t *testing.T) {
	if !library.CurrentSubtitleNames().IsBeside("Show.S01E01", "Show.S01E01.en.srt") {
		t.Fatal("library has no subtitle rules after importing subtitles")
	}
}
