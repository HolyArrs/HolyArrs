package model

import "strings"

// MapPath turns a path a Downloader reports into the same place as HolyArrs
// sees it, by the first folder mapping whose Theirs folder holds it. A
// Windows path (a drive letter or backslashes, as from qBittorrent on a
// Windows host) matches whatever its case and its separators; the rest of
// it takes the separators of Ours. A path no mapping holds is returned as
// it is. ok is false for a path with a ".." part: a Downloader never
// reports one, and it could lead out of the mapped folder.
func MapPath(p string, maps []PathMap) (string, bool) {
	if hasDotDot(p) {
		return p, false
	}
	for _, m := range maps {
		theirs := strings.TrimRight(strings.TrimSpace(m.Theirs), `/\`)
		ours := strings.TrimRight(strings.TrimSpace(m.Ours), `/\`)
		if theirs == "" {
			continue
		}
		rest, ok := cutFolder(p, theirs)
		if !ok {
			continue
		}
		if windowsPath(theirs) {
			rest = strings.ReplaceAll(rest, `\`, "/")
		}
		if windowsPath(ours) && !strings.Contains(ours, "/") {
			rest = strings.ReplaceAll(rest, "/", `\`)
		}
		return ours + rest, true
	}
	return p, true
}

// cutFolder says whether p is folder or inside it, and returns the rest of
// p after folder (with its leading separator). Windows folders compare
// without case and with either separator; others exactly.
func cutFolder(p, folder string) (string, bool) {
	win := windowsPath(folder)
	pp, ff := p, folder
	if win {
		pp, ff = strings.ReplaceAll(p, `\`, "/"), strings.ReplaceAll(folder, `\`, "/")
	}
	n := len(ff)
	if len(pp) < n {
		return "", false
	}
	same := pp[:n] == ff
	if win {
		same = strings.EqualFold(pp[:n], ff)
	}
	if !same {
		return "", false
	}
	if len(pp) == n {
		return "", true
	}
	if pp[n] == '/' || (!win && pp[n] == '\\') {
		return p[n:], true
	}
	return "", false
}

// windowsPath says whether p reads as a Windows path: a drive letter ("D:")
// or a backslash.
func windowsPath(p string) bool {
	if len(p) >= 2 && p[1] == ':' && (p[0] >= 'a' && p[0] <= 'z' || p[0] >= 'A' && p[0] <= 'Z') {
		return true
	}
	return strings.Contains(p, `\`)
}

// hasDotDot says whether a path has a ".." part (with either separator).
func hasDotDot(p string) bool {
	for _, part := range strings.FieldsFunc(p, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == ".." {
			return true
		}
	}
	return false
}
