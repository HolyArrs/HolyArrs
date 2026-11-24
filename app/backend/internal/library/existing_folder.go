package library

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

var reIDTag = regexp.MustCompile(`(?i)[\[{(]\s*(?:tmdb|tvdb|imdb|tvmaze)(?:id)?[-_=: ]?[a-z]*\d+\s*[\]})]`)

// reIDTagParts reads the source and id out of one id tag.
var reIDTagParts = regexp.MustCompile(`(?i)[\[{(]\s*(tmdb|tvdb|imdb|tvmaze)(?:id)?[-_=: ]?([a-z]*\d+)\s*[\]})]`)

// FolderFacts is what an add knows about a title when it looks for the
// folder already holding it.
type FolderFacts struct {
	Name  string            // the main name
	Year  int               // 0 when unknown
	Other []string          // original and other names, tried after Name
	IDs   map[string]string // source ("tmdb", "tvdb", "imdb") to id
}

// ExistingFolder is FindFolder with only a name and year.
func ExistingFolder(root, want, name string, year int, taken func(string) bool) string {
	return FindFolder(root, want, FolderFacts{Name: name, Year: year}, taken)
}

// FindFolder picks the folder already in root that holds the title, so an
// add reads the files already there instead of making a fresh folder named
// by the naming setup. It returns want when that folder exists, or when no
// folder, or more than one, matches; taken folders (by name as given,
// parents included) are skipped.
//
// want may sit under parent folders ("M/Name (1999)"); a parent spelled
// with other letter case on disk ("m/") is used as it is. Inside, in order:
// a folder whose id tag is the title's id; then by name (the main name, the
// name the naming setup put in want, other names), same year or no year;
// then one year off. Names compare ignoring case, accents and punctuation
// on every system. A folder tagged with another title's id never matches.
func FindFolder(root, want string, f FolderFacts, taken func(string) bool) string {
	parent, base := path.Split(want)
	parent = existingParent(root, parent)
	under := taken
	if parent != "" && taken != nil {
		under = func(d string) bool { return taken(parent + d) }
	}
	ents, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(parent)))
	if err != nil {
		return parent + base
	}
	fold := FoldsCase(filepath.Join(root, filepath.FromSlash(parent)))
	var dirs []string
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		if sameFolderName(e.Name(), base, fold) {
			return parent + base
		}
		if under == nil || !under(e.Name()) {
			dirs = append(dirs, e.Name())
		}
	}
	return parent + pickFolder(dirs, base, f)
}

// existingParent returns parent ("M/" or "") with each folder replaced by
// the one folder on disk that differs from it only in letter case, when
// the folder as spelled is missing.
func existingParent(root, parent string) string {
	if parent == "" {
		return ""
	}
	parts := strings.Split(strings.Trim(parent, "/"), "/")
	dir := root
	for i, p := range parts {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			ents, err := os.ReadDir(dir)
			if err != nil {
				return parent
			}
			hit := ""
			for _, e := range ents {
				if e.IsDir() && strings.EqualFold(e.Name(), p) {
					if hit != "" {
						return parent
					}
					hit = e.Name()
				}
			}
			if hit == "" {
				return parent
			}
			parts[i] = hit
		}
		dir = filepath.Join(dir, parts[i])
	}
	return strings.Join(parts, "/") + "/"
}

func folderIDs(dir string) map[string]string {
	var ids map[string]string
	for _, m := range reIDTagParts.FindAllStringSubmatch(dir, -1) {
		if ids == nil {
			ids = map[string]string{}
		}
		ids[strings.ToLower(m[1])] = strings.ToLower(m[2])
	}
	return ids
}

func pickFolder(dirs []string, want string, f FolderFacts) string {
	// Id tags: the title's own id wins; a folder tagged for another title
	// is left out of the name match.
	var byID, free []string
	for _, d := range dirs {
		ids := folderIDs(d)
		mine, other := false, false
		for src, id := range ids {
			if v := strings.ToLower(strings.TrimSpace(f.IDs[src])); v != "" {
				if v == id {
					mine = true
				} else {
					other = true
				}
			}
		}
		switch {
		case mine && !other:
			byID = append(byID, d)
		case !other:
			free = append(free, d)
		}
	}
	if len(byID) == 1 {
		return byID[0]
	}
	if len(byID) > 1 {
		return want
	}

	names := []string{f.Name}
	if local, _ := FolderName(strings.TrimSpace(reIDTag.ReplaceAllString(want, ""))); local != "" {
		names = append(names, local)
	}
	names = append(names, f.Other...)
	seen := map[string]bool{}
	for _, n := range names {
		c := releasename.CleanName(n)
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		if got, ok := matchName(free, n, c, f.Year, false); ok {
			return orWant(got, want)
		}
	}
	// One year off (a release dated by another country): only when the
	// main name finds exactly one folder.
	if f.Year != 0 {
		if c := releasename.CleanName(f.Name); c != "" {
			if got, ok := matchName(free, f.Name, c, f.Year, true); ok {
				return orWant(got, want)
			}
		}
	}
	return want
}

func orWant(got, want string) string {
	if got == "" {
		return want
	}
	return got
}

// matchName looks for folders named name. ok is true when it settled the
// answer: one folder ("" with ok means more than one matched, so no pick).
// A folder spelled like the name wins over one spelled as release names
// write it ("Schitts Creek", "SWAT").
func matchName(dirs []string, name, clean string, year int, offByOne bool) (string, bool) {
	var exact, loose, nearExact, nearLoose []string
	for _, dir := range dirs {
		n, y := FolderName(strings.TrimSpace(reIDTag.ReplaceAllString(dir, "")))
		ex, lo := &exact, &loose
		if c := releasename.CleanName(n); c != clean {
			if !releasename.SameName(name, c) {
				continue
			}
			ex, lo = &nearExact, &nearLoose
		}
		switch {
		case offByOne:
			if y != 0 && (y == year-1 || y == year+1) {
				*ex = append(*ex, dir)
			}
		case y == year:
			*ex = append(*ex, dir)
		case y == 0 || year == 0:
			*lo = append(*lo, dir)
		}
	}
	for _, set := range [][2][]string{{exact, loose}, {nearExact, nearLoose}} {
		switch {
		case len(set[0]) == 1:
			return set[0][0], true
		case len(set[0]) == 0 && len(set[1]) == 1:
			return set[1][0], true
		case len(set[0])+len(set[1]) > 0:
			return "", true
		}
	}
	return "", false
}

// TitleFacts gathers what FindFolder uses from a title.
func TitleFacts(t *mediafiles.Title, year int) FolderFacts {
	f := FolderFacts{Name: t.Name, Year: year, IDs: t.SourceIDs}
	if t.OriginalName != nil {
		f.Other = append(f.Other, *t.OriginalName)
	}
	f.Other = append(f.Other, t.Aliases...)
	return f
}
