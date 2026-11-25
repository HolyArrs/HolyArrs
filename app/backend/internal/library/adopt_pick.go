package library

import (
	"os"
	"path"
	"path/filepath"
	"strings"
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
