package library

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// maxGroupDepth is how many grouping folders (letters, genres,
// collections) a movie folder may sit under.
const maxGroupDepth = 16

// reDiscFolder is a numbered disc or part folder: "Disc 1", "cd2",
// "Part 2", "Disc 1 - Feature".
var reDiscFolder = regexp.MustCompile(`^(disc|disk|cd|dvd|part)( ?[0-9]+( .*)?| [a-z])$`)

// extrasFolders are the whole folder names that hold a movie's extras.
var extrasFolders = map[string]bool{"extras": true, "extra": true, "bonus": true, "bonus features": true, "making of": true,
	"behind the scenes": true, "deleted scenes": true, "featurettes": true, "featurette": true, "specials": true,
	"special features": true, "samples": true, "sample": true, "trailers": true, "trailer": true, "interviews": true,
	"subs": true, "subtitles": true, "feature": true, "other": true, "others": true, "bdmv": true, "video ts": true,
	"audio ts": true, "certificate": true, "backup": true, "images": true, "screenshots": true, "cover art screens": true}

// partFolder says a subfolder belongs to the movie folder around it
// (extras, discs, bonus material) rather than being a movie of its own.
func partFolder(name string) bool {
	n := strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if r == '.' || r == '_' || r == '-' {
			return ' '
		}
		return r
	}, strings.ToLower(name))), " ")
	return extrasFolders[n] || reDiscFolder.MatchString(n) || strings.HasPrefix(n, "subtitles ")
}

// movieFolders lists movie folders and individual collection videos as
// relative paths, sorted. Grouping folders are replaced by their contents.
func movieFolders(root string, ents []os.DirEntry) []string {
	return movieCandidates(root, ents, nil)
}

func movieCandidates(root string, ents []os.DirEntry, shared map[string]bool) []string {
	// Group detection and walking inspect the same folders. Read each once,
	// particularly on network shares where directory round trips are costly.
	type listing struct {
		entries []os.DirEntry
		err     error
	}
	cache := map[string]listing{root: {entries: ents}}
	read := func(dir string) ([]os.DirEntry, error) {
		if got, ok := cache[dir]; ok {
			return got.entries, got.err
		}
		es, err := os.ReadDir(dir)
		cache[dir] = listing{es, err}
		return es, err
	}
	out := collectionEntries(ents, shared["."])
	ancestors := sharedAncestors(shared)
	var walk func(rel string, ents []os.DirEntry, depth int)
	walk = func(rel string, ents []os.DirEntry, depth int) {
		for _, e := range ents {
			func() {
				// A completed top-level group will not be visited again. Do not
				// retain every directory listing in the library until scan end.
				if depth == 0 {
					defer func() { clear(cache) }()
				}
				if !ScanWants(e, "") || depth > 0 && !hasYearOrTag(e.Name()) && partFolder(e.Name()) {
					return
				}
				p := path.Join(rel, e.Name())
				dir := filepath.Join(root, filepath.FromSlash(p))
				subs, _ := read(dir)
				if files := collectionEntries(subs, shared[p]); len(files) > 0 {
					for _, f := range files {
						out = append(out, path.Join(p, f))
					}
					if depth < maxGroupDepth {
						walk(p, subs, depth+1)
					}
					return
				}
				if ancestors[p] && depth < maxGroupDepth {
					walk(p, subs, depth+1)
					return
				}
				if subs, ok := groupOfRead(dir, e.Name(), depth, read); ok {
					walk(p, subs, depth+1)
					return
				}
				out = append(out, p)
			}()
		}
	}
	walk("", ents, 0)
	sort.Strings(out)
	return out
}

// groupOf reads a folder and says whether it groups movie folders: a
// subfolder with a year or TMDB tag, one that groups movies itself, or
// two or more holding their own videos. Disc, extras and bonus folders
// never count, so a movie with its videos only in those stays whole.
func groupOf(dir, name string, depth int) ([]os.DirEntry, bool) {
	return groupOfRead(dir, name, depth, os.ReadDir)
}

func groupOfRead(dir, name string, depth int, read func(string) ([]os.DirEntry, error)) ([]os.DirEntry, bool) {
	if depth >= maxGroupDepth || hasYearOrTag(name) && !collectionName(name) {
		return nil, false
	}
	ents, err := read(dir)
	if err != nil {
		return nil, false
	}
	withVideo := 0
	for _, e := range ents {
		if !e.IsDir() && videoExt[strings.ToLower(filepath.Ext(e.Name()))] {
			return nil, false
		}
	}
	for _, e := range ents {
		if !ScanWants(e, "") {
			continue
		}
		if hasYearOrTag(e.Name()) {
			return ents, true
		}
		if partFolder(e.Name()) {
			continue
		}
		sub := filepath.Join(dir, e.Name())
		subEntries, _ := read(sub)
		if len(collectionEntries(subEntries, false)) > 0 {
			return ents, true
		}
		if _, ok := groupOfRead(sub, e.Name(), depth+1, read); ok {
			return ents, true
		}
		if entriesHaveVideo(subEntries) {
			withVideo++
		}
	}
	return ents, withVideo >= 2
}

// A year range on a collection describes its contents, not one movie.
var collectionYears = regexp.MustCompile(`(?:19|20)\d{2}\s*[-–]\s*(?:19|20)\d{2}`)
var collectionWords = regexp.MustCompile(`(?i)\b(?:collection|trilogy|quadrilogy|anthology)\b`)

func collectionName(name string) bool {
	return collectionYears.MatchString(name) || strings.Contains(strings.ToLower(name), "collection") || collectionWords.MatchString(strings.ReplaceAll(name, "_", " "))
}

func hasYearOrTag(name string) bool {
	_, year := FolderName(name)
	return year != 0 || reTMDBTag.MatchString(name)
}

func entriesHaveVideo(ents []os.DirEntry) bool {
	for _, e := range ents {
		if !e.IsDir() && videoExt[strings.ToLower(filepath.Ext(e.Name()))] {
			return true
		}
	}
	return false
}

// afterIn is the folders of names that sort after after.
func afterIn(names []string, after string) []string {
	if after == "" {
		return names
	}
	return names[sort.Search(len(names), func(i int) bool { return names[i] > after }):]
}

// validFolder says folder is a path of plain folders inside a Library
// folder, written with forward slashes.
func validFolder(folder string) bool {
	if folder == "" || strings.Contains(folder, `\`) || !filepath.IsLocal(filepath.FromSlash(folder)) {
		return false
	}
	for _, seg := range strings.Split(folder, "/") {
		if seg == "" || seg == "." || seg == ".." || strings.HasPrefix(seg, ".") || seg != strings.TrimSpace(seg) {
			return false
		}
	}
	return true
}
