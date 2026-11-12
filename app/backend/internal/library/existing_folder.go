package library

import (
	"os"
	"regexp"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

var reIDTag = regexp.MustCompile(`(?i)[\[{(]\s*(?:tmdb|tvdb|imdb|tvmaze)(?:id)?[-_=: ]?[a-z]*\d+\s*[\]})]`)

// ExistingFolder picks the folder already in root that holds the title
// named name (year 0 when unknown), so an add reads the files already
// there instead of a fresh folder named by the naming setup. It returns
// want when that folder exists, or when no folder (or more than one)
// matches; taken folders, by name as given, are skipped.
func ExistingFolder(root, want, name string, year int, taken func(string) bool) string {
	ents, err := os.ReadDir(root)
	if err != nil {
		return want
	}
	fold := FoldsCase(root)
	clean := releasename.CleanName(name)
	if clean == "" {
		return want
	}
	var exact, loose []string
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		dir := e.Name()
		if sameFolderName(dir, want, fold) {
			return want
		}
		if taken != nil && taken(dir) {
			continue
		}
		n, y := FolderName(strings.TrimSpace(reIDTag.ReplaceAllString(dir, "")))
		if releasename.CleanName(n) != clean {
			continue
		}
		switch {
		case y == year:
			exact = append(exact, dir)
		case y == 0 || year == 0:
			loose = append(loose, dir)
		}
	}
	switch {
	case len(exact) == 1:
		return exact[0]
	case len(exact) == 0 && len(loose) == 1:
		return loose[0]
	}
	return want
}
