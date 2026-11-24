package library

import (
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// The naming Example shows what the French version word adds.
func TestNamingExampleFrenchVersion(t *testing.T) {
	ns := mediafiles.NamingSettings{FilePattern: "{title} ({year}) {release}{.french_version}", FolderPattern: "{title} ({year})"}
	file, _ := namingExamples(ns)
	if !strings.Contains(file, ".VFQ") {
		t.Fatalf("example %q has no French version", file)
	}
}
