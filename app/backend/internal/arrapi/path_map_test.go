package arrapi

import (
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// The queue's outputPath, which Unpackerr reads, maps a Windows
// Downloader's folder the same way the import does.
func TestOutputPath_MapsWindowsPaths(t *testing.T) {
	maps := []domain.PathMap{{Theirs: `D:\Downloads`, Ours: "/downloads"}}
	for p, want := range map[string]string{
		`D:\Downloads\holyarrs`: "/downloads/holyarrs",
		`d:\downloads\holyarrs`: "/downloads/holyarrs",
		"/other/x":              "/other/x",
	} {
		if got := mapPath(p, maps); got != want {
			t.Errorf("%s: %s, want %s", p, got, want)
		}
	}
}

// A reported folder with a ".." part never reaches Unpackerr.
func TestOutputPath_RefusesDotDot(t *testing.T) {
	if got := outputPath("/data/downloads/../../etc", []domain.PathMap{{Theirs: "/data/downloads", Ours: "/downloads"}}, nil); got != "" {
		t.Fatalf("got %q", got)
	}
}
