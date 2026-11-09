package importer

import (
	"path/filepath"
	"runtime"
	"strings"
)

// UnsafeFolder reports whether p, with symlinks followed, is a filesystem
// or drive root, or overlaps the data folder dataDir (either holds the
// other). Such a folder must not be a Library or import folder: HolyArrs
// would move, rename or read its own database and keys. dataDir may be
// empty (only the root check runs then).
func UnsafeFolder(p, dataDir string) bool {
	p = realPath(p)
	if filepath.Dir(p) == p {
		return true
	}
	if dataDir == "" {
		return false
	}
	return pathsOverlap(p, realPath(dataDir))
}

func realPath(p string) string {
	p = filepath.Clean(p)
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

func pathsOverlap(a, b string) bool {
	if runtime.GOOS == "windows" {
		a, b = strings.ToLower(a), strings.ToLower(b)
	}
	if a == b {
		return true
	}
	sep := string(filepath.Separator)
	return strings.HasPrefix(a, strings.TrimSuffix(b, sep)+sep) || strings.HasPrefix(b, strings.TrimSuffix(a, sep)+sep)
}
