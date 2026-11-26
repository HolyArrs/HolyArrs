package library

import (
	"path"
	"strings"
	"unicode"
)

// movieExtraIn is MovieExtra for a video in a movie folder. A file there
// named like an extras folder ("Deleted.Scenes.mkv") is an extra too, and
// a file whose name is just the movie's own title (as in
// "Heat-Interview.mkv" for "Heat Interview (2001)") is the movie.
func movieExtraIn(folder, rel string) bool {
	if strings.Contains(rel, "/") {
		return MovieExtra(rel)
	}
	if !MovieExtra(rel) && !extraOnlyFolders[folderWords(scanName(rel))] {
		return false
	}
	return plainWords(strings.ToLower(scanName(rel))) != titleKey(path.Base(folder))
}

// plainWords keeps only the letters and digits of s, one space apart.
func plainWords(s string) string {
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }), " ")
}

// titleKey reduces a folder or file name to its title words for comparing:
// lower case, no year, no punctuation, and "Big Short, The" read as
// "the big short".
func titleKey(name string) string {
	title, _ := FolderName(name)
	if title == "" {
		title = name
	}
	t := strings.ToLower(strings.TrimSpace(title))
	for _, a := range []string{"the", "a", "an"} {
		if rest, ok := strings.CutSuffix(t, ", "+a); ok {
			t = a + " " + rest
		}
	}
	return plainWords(t)
}

// extraWords are the words media servers put after a dash at the end of a
// video's name to mark one of the movie's extras, as in
// "Heat (1995)-trailer.mkv". strongExtraWords never end a real title, so
// they also count after a dot or an underscore ("Heat.1995.trailer.mkv").
var (
	extraWords       = []string{"trailer", "featurette", "behindthescenes", "deleted", "interview", "scene", "short", "other", "sample", "clip", "extra"}
	strongExtraWords = map[string]bool{"trailer": true, "featurette": true, "behindthescenes": true, "sample": true}
)

// extraName says a video's base name (lower case, no extension) marks an
// extra: it is just "trailer" or "sample", or ends in an extras word after
// a separator, with more than "the", "a" or "an" before it, so dash-joined
// titles like "The-Interview" stay movies.
func extraName(base string) bool {
	if base == "trailer" || base == "sample" {
		return true
	}
	for _, w := range extraWords {
		stem, ok := strings.CutSuffix(base, w)
		stem = strings.TrimRight(stem, " ")
		if !ok || stem == "" {
			continue
		}
		seps := "-"
		if strongExtraWords[w] {
			seps = "-._"
		}
		if !strings.ContainsAny(stem[len(stem)-1:], seps) {
			continue
		}
		switch folderWords(stem) {
		case "", "the", "a", "an":
			continue
		}
		return true
	}
	return false
}
