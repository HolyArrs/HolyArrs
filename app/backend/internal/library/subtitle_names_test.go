package library

import (
	"os"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles"
)

func TestMain(m *testing.M) {
	UseSubtitleNames(subtitles.Names{})
	os.Exit(m.Run())
}
