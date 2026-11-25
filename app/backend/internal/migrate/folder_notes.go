package migrate

import "strings"

// unreachableFolders lists, once each and in order, the old folders this
// server could not reach, joined by ", ".
func unreachableFolders(rep *Report) string {
	var folders []string
	seen := map[string]bool{}
	for _, l := range rep.Lines {
		if l.Attention == AttnUnreachable && l.Folder != "" && !seen[l.Folder] {
			seen[l.Folder] = true
			folders = append(folders, l.Folder)
		}
	}
	return strings.Join(folders, ", ")
}

// missingFolders lists, once each and in order, the old Library folders
// not found on this server, joined by ", "; all is false when some
// not-found line names no folder.
func missingFolders(rep *Report) (string, bool) {
	var folders []string
	seen, all := map[string]bool{}, true
	for _, l := range rep.Lines {
		if l.Result != ResNotFound {
			continue
		}
		if l.missing == "" {
			all = false
		} else if !seen[l.missing] {
			seen[l.missing] = true
			folders = append(folders, l.missing)
		}
	}
	return strings.Join(folders, ", "), all
}
