package importer

import (
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// FollowLinks makes a symlink to a plain file count as that file:
// setups that stream media through a mounted filesystem keep their Library
// and downloads as links. Off by default; HOLYARRS_FOLLOW_LINKS=1 turns it on.
var FollowLinks = os.Getenv("HOLYARRS_FOLLOW_LINKS") == "1"

// LinkedFile returns the target's info when p is a symlink to a plain file
// and links are followed.
func LinkedFile(p string) (os.FileInfo, bool) {
	if !FollowLinks && !linkPairsOn.Load() {
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

// linkPairsOn is set once a Downloader has a path pair that places links:
// the Library then holds links, and they must count as its files.
var linkPairsOn atomic.Bool

// NoteLinkPairs turns link following on when any of the Downloaders has a
// path pair that places links. It never turns it off again: links already
// placed stay in the Library.
func NoteLinkPairs(drs []domain.Downloader) {
	for _, d := range drs {
		if domain.HasLinkPair(d.PathMap) {
			linkPairsOn.Store(true)
			return
		}
	}
}

// markLinks marks the steps whose download sits in a path pair that places
// links. Such a file is never moved, hardlinked or copied.
func markLinks(plan []mediafiles.PlanStep, dr domain.Downloader) {
	if !domain.HasLinkPair(dr.PathMap) {
		return
	}
	NoteLinkPairs([]domain.Downloader{dr})
	for i := range plan {
		st := &plan[i]
		if st.Method == "hardlink" && domain.InLinkPair(st.SourceFile, dr.PathMap) {
			st.Symlink, st.Move = true, false
		}
	}
}

// placeSymlink puts a link at target that holds src's absolute path. src
// itself may be a link; the new link points at the name the Downloader
// gave, so a remounted drive still resolves.
func placeSymlink(s mediafiles.PlanStep) mediafiles.PlaceResult {
	r := mediafiles.PlaceResult{SourceFile: s.SourceFile, PlacedBy: "none"}
	src, err := filepath.Abs(s.SourceFile)
	if err == nil {
		_, err = os.Stat(src)
	}
	if err != nil {
		r.Outcome, r.Error = "failed", placeWhy(err, err.Error(), s.SourceFile)
		return r
	}
	if to, err := os.Readlink(s.TargetPath); err == nil {
		if to == src {
			r.Outcome, r.PlacedBy, r.Linked = "placed", "copy", true
			return r
		}
		r.Outcome, r.Error = "refused", targetTakenWhy
		return r
	} else if _, err := os.Lstat(s.TargetPath); err == nil {
		r.Outcome, r.Error = "refused", targetTakenWhy
		return r
	}
	if err := os.MkdirAll(filepath.Dir(s.TargetPath), 0o777); err != nil {
		r.Outcome, r.Error = "failed", placeWhy(err, err.Error(), s.SourceFile)
		return r
	}
	if err := os.Symlink(src, s.TargetPath); err != nil {
		r.Outcome, r.Error = "failed", "could not place a link: "+err.Error()
		return r
	}
	// placed_by has no link kind; Linked tells it apart from a real copy.
	r.Outcome, r.PlacedBy, r.Linked = "placed", "copy", true
	return r
}
