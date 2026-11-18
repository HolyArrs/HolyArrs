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
	// A folder spelled like the name wins; one spelled as release names
	// write it ("Schitts Creek", "SWAT") counts only when none is.
	var exact, loose, nearExact, nearLoose []string
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
		ex, lo := &exact, &loose
		if c := releasename.CleanName(n); c != clean {
			if !releasename.SameName(name, c) {
				continue
			}
			ex, lo = &nearExact, &nearLoose
		}
		switch {
		case y == year:
			*ex = append(*ex, dir)
		case y == 0 || year == 0:
			*lo = append(*lo, dir)
		}
	}
	for _, set := range [][2][]string{{exact, loose}, {nearExact, nearLoose}} {
		switch {
		case len(set[0]) == 1:
			return set[0][0]
		case len(set[0]) == 0 && len(set[1]) == 1:
			return set[1][0]
		case len(set[0])+len(set[1]) > 0:
			return want
		}
	}
	return want
}
