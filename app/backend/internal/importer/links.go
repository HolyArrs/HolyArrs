package importer

import (
	"os"
	"path/filepath"
	"strings"
)

// FollowLinks makes a symlink to a plain file count as that file:
// setups that stream media through a mounted filesystem keep their Library
// and downloads as links. Off by default; HOLYARRS_FOLLOW_LINKS=1 turns it on.
var FollowLinks = os.Getenv("HOLYARRS_FOLLOW_LINKS") == "1" || len(LinkPaths) > 0

// LinkPaths are folders on a mounted streaming drive. A downloaded file
// inside one is placed in the Library as a link to it, never copied, and
// the download is never deleted afterwards, because the link needs it.
// HOLYARRS_LINK_PATHS lists the folders, separated by commas. Setting it also
// turns on FollowLinks, so the Library scan reads the links it places.
var LinkPaths = parseLinkPaths(os.Getenv("HOLYARRS_LINK_PATHS"))

func parseLinkPaths(list string) []string {
	var out []string
	for _, p := range strings.Split(list, ",") {
		if p = strings.TrimSpace(p); filepath.IsAbs(p) {
			out = append(out, filepath.Clean(p))
		}
	}
	return out
}

// InLinkPath reports whether p is inside one of the LinkPaths.
func InLinkPath(p string) bool {
	p = filepath.Clean(p)
	for _, root := range LinkPaths {
		if under(root, p) {
			return true
		}
	}
	return false
}

// LinkedFile returns the target's info when p is a symlink to a plain file
// and links are followed.
func LinkedFile(p string) (os.FileInfo, bool) {
	if !FollowLinks {
		return nil, false
	}
	if fi, err := os.Lstat(p); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		return nil, false
	}
	fi, err := os.Stat(p)
	if err != nil || !fi.Mode().IsRegular() {
		return nil, false
	}
	return fi, true
}

// InsideRealFile is InsideReal for a file: a followed link only needs its
// folder inside root, since its target lives elsewhere by design.
func InsideRealFile(root, p string) bool {
	if _, ok := LinkedFile(p); ok {
		return insideReal(root, filepath.Dir(p))
	}
	return insideReal(root, p)
}

// placeLink puts a new link at target pointing where src's link resolves.
func placeLink(src, target string) error {
	real, err := filepath.EvalSymlinks(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o777); err != nil {
		return err
	}
	return os.Symlink(real, target)
}
