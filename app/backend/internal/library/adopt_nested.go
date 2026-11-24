package library

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
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
	n := folderWords(name)
	return extrasFolders[n] || reDiscFolder.MatchString(n) || strings.HasPrefix(n, "subtitles ")
}

// folderWords lowers a folder name and turns dots, dashes and
// underscores into single spaces, so "Behind.The.Scenes" reads as
// "behind the scenes".
func folderWords(name string) string {
	return strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if r == '.' || r == '_' || r == '-' {
			return ' '
		}
		return r
	}, strings.ToLower(name))), " ")
}

// extraOnlyFolders hold nothing but a movie's extras. It is extrasFolders
// without the folders that can hold the movie itself: disc structures
// (BDMV, VIDEO_TS), "Feature", subtitles and backups.
var extraOnlyFolders = map[string]bool{"extras": true, "extra": true, "bonus": true, "bonus features": true, "making of": true,
	"behind the scenes": true, "deleted scenes": true, "featurettes": true, "featurette": true, "specials": true,
	"special features": true, "samples": true, "sample": true, "trailers": true, "trailer": true, "interviews": true,
	"interview": true, "scenes": true, "shorts": true, "other": true, "others": true}

// MovieExtra says a video in a movie folder is one of the movie's extras,
// not the movie: it sits in an extras folder, or its name ends the way
// media servers mark extras, or it is named just "trailer" or "sample". rel is the
// video's path inside the movie folder, with forward slashes.
func MovieExtra(rel string) bool {
	parts := strings.Split(rel, "/")
	for _, dir := range parts[:len(parts)-1] {
		if extraOnlyFolders[folderWords(dir)] {
			return true
		}
	}
	name := parts[len(parts)-1]
	base := strings.ToLower(strings.TrimSuffix(name, path.Ext(name)))
	return extraName(base)
}

// mainMovieVideo picks the movie's own file in a movie folder: the
// biggest video that is not an extra. A folder that holds only extras has
// no movie file, so a trailer or a featurette is never taken as the movie
// and later upgraded or deleted as one. Extras do not count toward the
// read limit, so a folder with many of them still shows its movie.
func mainMovieVideo(lib, folder string) (AdoptVideo, bool) {
	vids := listVideosWhere(lib, folder, maxScanVideos, func(rel string) bool { return !movieExtraIn(folder, rel) })
	if len(vids) == 0 {
		return AdoptVideo{}, false
	}
	return vids[0], true
}

// movieFolders lists movie folders and individual collection videos as
// relative paths, sorted. Grouping folders are replaced by their contents.
func movieFolders(root string, ents []os.DirEntry) []string {
	out, _ := movieFoldersLoose(root, ents)
	return out
}

// movieFoldersLoose is movieFolders, also naming the folders (and "" for
// the Library folder itself) that hold two or more movies as loose
// files, for Tidy my folders.
func movieFoldersLoose(root string, ents []os.DirEntry) (out, loose []string) {
	return movieCandidatesLoose(root, ents, nil)
}

func movieCandidates(root string, ents []os.DirEntry, shared map[string]bool) []string {
	out, _ := movieCandidatesLoose(root, ents, shared)
	return out
}

// movieCandidatesLoose walks the Library folder for movie folders. A
// collection folder holding several different movies as loose files offers
// each file on its own; loose files directly in the Library folder are offered
// by looseGroups instead.
func movieCandidatesLoose(root string, ents []os.DirEntry, shared map[string]bool) (out, loose []string) {
	if looseMovies(ents) >= 2 {
		loose = append(loose, "")
	}
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
	ancestors := sharedAncestors(shared)
	var walk func(rel string, ents []os.DirEntry, depth int, inColl bool)
	walk = func(rel string, ents []os.DirEntry, depth int, inColl bool) {
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
				coll := inColl || collectionName(e.Name())
				dir := filepath.Join(root, filepath.FromSlash(p))
				subs, _ := read(dir)
				// Only a folder in a collection (or already shared) offers
				// its movies one by one; a genre folder of loose movies is left
				// to Tidy my folders, as before.
				if files := collectionEntries(subs, shared[p]); len(files) > 0 && (shared[p] || coll) {
					for _, f := range files {
						out = append(out, path.Join(p, f))
					}
					if depth < maxGroupDepth {
						walk(p, subs, depth+1, coll)
					}
					return
				}
				if ancestors[p] && depth < maxGroupDepth {
					walk(p, subs, depth+1, coll)
					return
				}
				if subs, ok := groupOfRead(dir, e.Name(), depth, read); ok {
					if looseMovies(subs) >= 2 {
						loose = append(loose, p)
					}
					walk(p, subs, depth+1, coll)
					return
				}
				out = append(out, p)
			}()
		}
	}
	walk("", ents, 0, false)
	sort.Strings(out)
	return out, loose
}

// looseMovies counts the different movies among the videos directly in a
// folder, read from file names with a year: "Alien (1979).mkv" and
// "Heat.1995.1080p.mkv" are two, a cut or part of one movie is one.
func looseMovies(ents []os.DirEntry) int {
	seen := map[string]bool{}
	for _, e := range ents {
		if e.IsDir() || !videoExt[strings.ToLower(filepath.Ext(e.Name()))] {
			continue
		}
		name, year := FolderName(strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())))
		if year != 0 {
			seen[strings.ToLower(name)+"|"+strconv.Itoa(year)] = true
		}
	}
	return len(seen)
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
	// Videos directly in it make it a movie folder, unless they are two
	// or more different movies: a genre folder of loose files.
	if looseMovies(ents) >= 2 {
		return ents, true
	}
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

func videoIn(dir string) bool {
	ents, _ := os.ReadDir(dir)
	return entriesHaveVideo(ents)
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

// scanName is the name a scan matches: a folder name as it is, a loose
// video's file name without its extension.
func scanName(name string) string {
	if videoExt[strings.ToLower(path.Ext(name))] {
		return strings.TrimSuffix(name, path.Ext(name))
	}
	return name
}

// looseGroups lists the movies that sit as loose videos directly in the
// Library folder, each under its first file name, with every cut
// of it (the same name and year) biggest first. Samples, hidden files and
// links that lead nowhere are left out. A flat folder is a fine way to keep
// movies: these are offered like movie folders.
func looseGroups(root string, ents []os.DirEntry) map[string][]AdoptVideo {
	byKey := map[string]string{}
	out := map[string][]AdoptVideo{}
	for _, e := range ents {
		n := e.Name()
		if e.IsDir() || strings.HasPrefix(n, ".") || !videoExt[strings.ToLower(filepath.Ext(n))] || sampleName(n) || MovieExtra(n) {
			continue
		}
		p := filepath.Join(root, n)
		var size int64
		if linked, isLink := importer.LinkedFile(p); isLink {
			size = linked.Size()
		} else if fi, err := e.Info(); err == nil && fi.Mode().IsRegular() {
			size = fi.Size()
		} else {
			continue
		}
		name, year := FolderName(scanName(n))
		key := strings.ToLower(scanName(n))
		if year != 0 {
			key = strings.ToLower(name) + "|" + strconv.Itoa(year)
		}
		first, ok := byKey[key]
		if !ok || n < first {
			if ok {
				out[n] = out[first]
				delete(out, first)
			}
			byKey[key], first = n, n
		}
		out[first] = append(out[first], AdoptVideo{Path: n, Size: size})
	}
	for k := range out {
		sort.SliceStable(out[k], func(i, j int) bool { return out[k][i].Size > out[k][j].Size })
	}
	return out
}

// sampleName says a video's file name marks it as a sample.
func sampleName(n string) bool {
	base := strings.ToLower(strings.TrimSuffix(n, filepath.Ext(n)))
	for _, w := range strings.FieldsFunc(base, func(r rune) bool { return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9') }) {
		if w == "sample" {
			return true
		}
	}
	return false
}

// SuggestFlat says a movie Library folder looks flat: three or more
// different movies sit as loose files directly in it, more than there are
// movie folders. It only suggests; nothing is switched or moved.
func SuggestFlat(looseMovies, folders int) bool {
	return looseMovies >= 3 && looseMovies > folders
}
