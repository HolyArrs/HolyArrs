package library

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
)

// looseOwner is the movie one of a loose candidate's videos belongs to.
func looseOwner(known map[string]int64, c AdoptCandidate, fold bool) (int64, bool) {
	for _, v := range c.Videos {
		if id, ok := known[FolderKey(v.Path, fold)]; ok {
			return id, true
		}
	}
	return 0, false
}

// mainCandidateVideo is mainMovieVideo for an adopt candidate that may be a
// single video file (a collection pack) or loose videos in the Library folder.
func mainCandidateVideo(root, rel string, vids []AdoptVideo, loose bool) (AdoptVideo, bool) {
	if loose {
		if len(vids) > 0 {
			return vids[0], true
		}
		return AdoptVideo{}, false
	}
	fi, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel)))
	if err == nil && fi.Mode().IsRegular() && videoExt[strings.ToLower(path.Ext(rel))] {
		if MovieExtra(path.Base(rel)) {
			return AdoptVideo{}, false
		}
		return AdoptVideo{Path: rel, Size: fi.Size()}, true
	}
	return mainMovieVideo(root, rel)
}

// NamedMovieVideo is the video called name directly in a movie folder,
// when it is a plain file (or, where links are followed, a link to one, as
// listing the folder counts it) that is not a sample or an extra. A name
// with either separator is refused, so a backslash can never leave the
// folder on Windows.
func NamedMovieVideo(root, folder, name string) (AdoptVideo, bool) {
	rel := path.Join(folder, name)
	if name == "" || strings.ContainsAny(name, `/\`) || path.Base(rel) != name || path.Dir(rel) != path.Clean(folder) {
		return AdoptVideo{}, false
	}
	local := filepath.FromSlash(rel)
	if !filepath.IsLocal(local) {
		return AdoptVideo{}, false
	}
	p := filepath.Join(root, local)
	fi, err := os.Lstat(p)
	if err == nil && !fi.Mode().IsRegular() {
		if linked, ok := importer.LinkedFile(p); ok {
			fi = linked
		}
	}
	if err != nil || !fi.Mode().IsRegular() || !videoExt[strings.ToLower(path.Ext(name))] || sampleVideo(name) || MovieExtra(name) {
		return AdoptVideo{}, false
	}
	return AdoptVideo{Path: rel, Size: fi.Size()}, true
}

// RecordedMovieVideo is NamedMovieVideo for the path an old app recorded
// in a movie folder, with forward slashes, which may name a file in a
// subfolder ("Disc1/Feature.mkv"), as Radarr scans them: only where
// listing the folder reaches (no ".", "..", hidden, system, sample or
// linked folder, at most maxScanDepth folders down), inside the folder on
// disk, and never an extra by its folders or its name. A backslash is
// refused.
func RecordedMovieVideo(root, folder, rel string) (AdoptVideo, bool) {
	dir, name := path.Split(rel)
	if dir == "" {
		return NamedMovieVideo(root, folder, rel)
	}
	if strings.Contains(rel, `\`) || path.Clean(rel) != rel || !filepath.IsLocal(filepath.FromSlash(rel)) || strings.Count(rel, "/") > maxScanDepth {
		return AdoptVideo{}, false
	}
	// The whole path below root is checked once more, so neither the
	// folder nor any part of rel can leave root.
	full := filepath.FromSlash(path.Join(folder, rel))
	if !filepath.IsLocal(full) {
		return AdoptVideo{}, false
	}
	parts := strings.Split(filepath.ToSlash(full), "/")
	depth := strings.Count(path.Clean(folder), "/") + 1
	if path.Clean(folder) == "." {
		depth = 0
	}
	// Listing the folder never follows a linked folder, so neither does this.
	at := root
	for i, d := range parts[:len(parts)-1] {
		at = filepath.Join(at, d)
		if i < depth {
			continue
		}
		if strings.HasPrefix(d, ".") || NasSystemDir(d) || strings.EqualFold(d, "sample") || strings.EqualFold(d, "samples") {
			return AdoptVideo{}, false
		}
		if fi, err := os.Lstat(at); err != nil || !fi.IsDir() {
			return AdoptVideo{}, false
		}
	}
	dir = strings.TrimSuffix(dir, "/")
	v, ok := NamedMovieVideo(root, path.Join(folder, dir), name)
	if !ok || MovieExtra(rel) || !importer.InsideRealFile(filepath.Join(root, filepath.FromSlash(folder)), filepath.Join(root, filepath.FromSlash(v.Path))) {
		return AdoptVideo{}, false
	}
	return v, true
}
