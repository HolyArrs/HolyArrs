package importer

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// oneEntry says whether a and b may be one directory entry reached by two
// spellings: through a symlinked or junctioned folder, or by a name that
// differs only in case on a disk that ignores case. Removing one would
// then remove both. When a folder cannot be read it says yes, so nothing
// is removed on a guess.
func oneEntry(a, b string) bool {
	pa, err1 := os.Stat(filepath.Dir(a))
	pb, err2 := os.Stat(filepath.Dir(b))
	if err1 != nil || err2 != nil {
		return true
	}
	return os.SameFile(pa, pb) && strings.EqualFold(filepath.Base(a), filepath.Base(b))
}

// moveChecked reports whether target is src itself (a hardlink) or a plain
// file with the same size and the same first and last bytes, so removing
// src after a move loses nothing. A source changed after the placing
// started may hold bytes the Library file lacks, so it stays.
func moveChecked(src, target string, started time.Time) bool {
	a, err1 := os.Stat(src)
	b, err2 := os.Stat(target)
	if err1 != nil || err2 != nil || !b.Mode().IsRegular() {
		return false
	}
	if !started.IsZero() && a.ModTime().After(started) {
		return false
	}
	return os.SameFile(a, b) || sameContent(src, target, a.Size())
}
