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
const maxGroupDepth = 3

// reDiscFolder is a numbered disc or part folder: "Disc 1", "cd2",
// "Part 2", "Disc 1 - Feature".
var reDiscFolder = regexp.MustCompile(`^(disc|disk|cd|dvd|part)( ?[0-9]+( .*)?| [a-z])$`)

// extrasFolders are the whole folder names that hold a movie's extras.
var extrasFolders = map[string]bool{"extras": true, "extra": true, "bonus": true, "bonus features": true, "making of": true,
	"behind the scenes": true, "deleted scenes": true, "featurettes": true, "featurette": true, "specials": true,
	"special features": true, "samples": true, "sample": true, "trailers": true, "trailer": true, "interviews": true,
	"subs": true, "subtitles": true, "feature": true, "other": true, "others": true, "bdmv": true, "video ts": true,
	"audio ts": true, "certificate": true, "backup": true}

// partFolder says a subfolder belongs to the movie folder around it
// (extras, discs, bonus material) rather than being a movie of its own.
func partFolder(name string) bool {
	n := folderWords(name)
	return extrasFolders[n] || reDiscFolder.MatchString(n)
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

// extraSuffixes are the name endings media servers read as a movie's
// extras, as in "Heat (1995)-trailer.mkv".
var extraSuffixes = []string{"-trailer", "-featurette", "-behindthescenes", "-deleted", "-interview", "-scene", "-short", "-other", "-extra"}

// MovieExtra says a video in a movie folder is one of the movie's extras,
// not the movie: it sits in an extras folder, or its name ends the way
// media servers mark extras, or it is named just "trailer". rel is the
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
	if base == "trailer" {
		return true
	}
	for _, s := range extraSuffixes {
		if strings.HasSuffix(base, s) {
			return true
		}
	}
	return false
}

// mainMovieVideo picks the movie's own file in a movie folder: the
// biggest video that is not an extra. A folder that holds only extras has
// no movie file, so a trailer or a featurette is never taken as the movie
// and later upgraded or deleted as one. Extras do not count toward the
// read limit, so a folder with many of them still shows its movie.
func mainMovieVideo(lib, folder string) (AdoptVideo, bool) {
	vids := listVideosWhere(lib, folder, maxScanVideos, func(rel string) bool { return !MovieExtra(rel) })
	if len(vids) == 0 {
		return AdoptVideo{}, false
	}
	return vids[0], true
}

// movieFolders lists the movie folders of a Library folder as paths
// under it, sorted. A folder with no year in its name, no video directly
// in it and folders of its own groups movies, and its folders are read in
// its place; it is never offered as a movie.
func movieFolders(root string, ents []os.DirEntry) []string {
	var out []string
	var walk func(rel string, ents []os.DirEntry, depth int)
	walk = func(rel string, ents []os.DirEntry, depth int) {
		for _, e := range ents {
			if !ScanWants(e, "") || depth > 0 && !hasYearOrTag(e.Name()) && partFolder(e.Name()) {
				continue
			}
			p := path.Join(rel, e.Name())
			if subs, ok := groupOf(filepath.Join(root, filepath.FromSlash(p)), e.Name(), depth); ok {
				walk(p, subs, depth+1)
				continue
			}
			out = append(out, p)
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
	if depth >= maxGroupDepth || hasYearOrTag(name) {
		return nil, false
	}
	ents, err := os.ReadDir(dir)
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
		if _, ok := groupOf(sub, e.Name(), depth+1); ok {
			return ents, true
		}
		if videoIn(sub) {
			withVideo++
		}
	}
	return ents, withVideo >= 2
}

func hasYearOrTag(name string) bool {
	_, year := FolderName(name)
	return year != 0 || reTMDBTag.MatchString(name)
}

// videoIn says a folder holds a video directly.
func videoIn(dir string) bool {
	ents, _ := os.ReadDir(dir)
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
