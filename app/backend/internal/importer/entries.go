package importer

import (
	"os"
	"path/filepath"
	"strings"
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
