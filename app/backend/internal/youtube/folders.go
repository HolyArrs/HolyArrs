package youtube

import (
	"os"
	"path"
	"path/filepath"
	"strings"
)

// leftovers are what a file browser or a NAS leaves in a folder; a folder
// holding nothing else counts as empty.
var leftovers = map[string]bool{".DS_Store": true, "Thumbs.db": true, "@eaDir": true}

// pruneEmpty removes the folder rel (forward slashes) under root when no
// video file is left in it, then its parents the same way. It never
// removes a person's own folder (the first level) or root itself, and never
// follows a link. A folder that cannot be read or removed just stays.
func pruneEmpty(root *os.Root, rel string) {
	for ; strings.Contains(rel, "/"); rel = path.Dir(rel) {
		if st, err := root.Lstat(rel); err != nil || !st.IsDir() {
			return
		}
		d, err := root.Open(rel)
		if err != nil {
			return
		}
		names, err := d.Readdirnames(-1)
		d.Close()
		if err != nil {
			return
		}
		for _, n := range names {
			if !leftovers[n] {
				return
			}
		}
		for _, n := range names {
			if err := root.RemoveAll(rel + "/" + n); err != nil {
				return
			}
		}
		if err := root.Remove(rel); err != nil {
			return
		}
	}
}

// pruneEmptyDir is pruneEmpty for the folder holding the file at an
// absolute path under root.
func pruneEmptyDir(root, file string) {
	if root == "" || file == "" {
		return
	}
	rel, err := filepath.Rel(root, filepath.Dir(file))
	if err != nil || !filepath.IsLocal(rel) {
		return
	}
	dir, err := os.OpenRoot(root)
	if err != nil {
		return
	}
	defer dir.Close()
	pruneEmpty(dir, filepath.ToSlash(rel))
}
