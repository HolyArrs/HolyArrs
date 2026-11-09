package library

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
)

// foldsCase says whether the filesystem holding dir treats names that
// differ only in case as one name. It probes dir, or the nearest parent
// whose name has a letter, by asking for that name with its case swapped;
// with nothing to probe it goes by the platform (Windows and macOS fold).
func FoldsCase(dir string) bool {
	for p := filepath.Clean(dir); ; {
		base := filepath.Base(p)
		if swapped := swapCase(base); swapped != base {
			fi, err := os.Stat(p)
			if err == nil {
				other, err := os.Stat(filepath.Join(filepath.Dir(p), swapped))
				return err == nil && os.SameFile(fi, other)
			}
		}
		parent := filepath.Dir(p)
		if parent == p {
			break
		}
		p = parent
	}
	return runtime.GOOS == "windows" || runtime.GOOS == "darwin"
}

func swapCase(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsUpper(r) {
			return unicode.ToLower(r)
		}
		return unicode.ToUpper(r)
	}, s)
}

// sameFolderName compares two folder names as the filesystem does.
func sameFolderName(a, b string, fold bool) bool {
	if fold {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// folderKey is a folder name as a map key for the filesystem.
func FolderKey(name string, fold bool) string {
	if fold {
		return strings.ToLower(name)
	}
	return name
}
