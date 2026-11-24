package library

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// reSeasonFolder is a folder a show keeps its episodes in: "Season 1",
// "S01", "Specials", "Staffel 2", "Saison 3", "Temporada 1".
var reSeasonFolder = regexp.MustCompile(`(?i)^(season|series|staffel|saison|temporada|stagione|seizoen|sezon|s)[ ._-]*\d+|^specials?$|^extras?$`)

// ShowFolders lists the show folders of a show Library folder as paths
// under it, sorted. A folder that groups shows (a letter such as O, 0-9
// or #, or a genre or language folder whose folders hold seasons) is read
// in its place, one level deep, and never offered as a show.
func ShowFolders(root string, ents []os.DirEntry) []string {
	var out []string
	for _, e := range ents {
		if !ScanWants(e, "") {
			continue
		}
		if subs, ok := showGroupOf(filepath.Join(root, e.Name()), e.Name()); ok {
			for _, s := range subs {
				out = append(out, path.Join(e.Name(), s))
			}
			continue
		}
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out
}

// AfterIn is the names sorted after after; all of them when after is "".
func AfterIn(names []string, after string) []string { return afterIn(names, after) }

// showGroupOf reads a folder and, when it groups show folders, returns
// their names.
func showGroupOf(dir, name string) ([]string, bool) {
	if hasYearOrTag(name) || reSeasonFolder.MatchString(name) {
		return nil, false
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, false
	}
	var subs []string
	for _, e := range ents {
		if !e.IsDir() && videoExt[strings.ToLower(filepath.Ext(e.Name()))] {
			return nil, false
		}
		if !ScanWants(e, "") {
			continue
		}
		if reSeasonFolder.MatchString(e.Name()) {
			return nil, false
		}
		subs = append(subs, e.Name())
	}
	if len(subs) == 0 {
		return nil, false
	}
	if letterGroup(name) {
		return subs, true
	}
	// Any other name groups shows only when a folder in it holds seasons.
	for _, s := range subs {
		inner, err := os.ReadDir(filepath.Join(dir, s))
		if err != nil {
			continue
		}
		for _, e := range inner {
			if e.IsDir() && reSeasonFolder.MatchString(e.Name()) {
				return subs, true
			}
		}
	}
	return nil, false
}

// letterGroup says a folder name is a first-letter folder: one letter or
// digit, 0-9 or #.
func letterGroup(name string) bool {
	return utf8.RuneCountInString(name) == 1 || name == "0-9" || name == "#"
}
