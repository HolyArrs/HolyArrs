package downloads

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// filesTarget is a torrent's data that HolyArrs may delete: rel inside root.
type filesTarget struct{ root, rel string }

// deleteAsk is what a Downloader that cannot delete files reports about one
// torrent, so HolyArrs can decide whether to delete them itself.
type deleteAsk struct {
	client   string   // the Downloader's name, used in the kept-files reasons
	content  string   // where the Downloader says the data is
	name     string   // the torrent's name
	others   []string // where every other torrent's data is
	pathMap  []domain.PathMap
	saveRoot string
	cats     []string // the categories whose own folder is never deleted
}

// safeDeleteTarget decides whether HolyArrs may delete a torrent's files after
// the Downloader lets go of it. It only says yes for a file or folder named
// like the torrent, inside the save folder or a path pair, that no other
// torrent uses and that is not in or around a Library folder. Otherwise it
// says why the files stay.
func safeDeleteTarget(a deleteAsk, keep []string) (filesTarget, string) {
	content := a.content
	if content == "" {
		return filesTarget{}, a.client + " did not say where they are"
	}
	if a.name == "" || a.name == "." || a.name == ".." || theirBase(content) != a.name {
		return filesTarget{}, content + " is not named like the torrent"
	}
	for _, oc := range a.others {
		if oc == "" {
			continue
		}
		if oc == content || strings.HasPrefix(oc, content+theirSep(content)) || strings.HasPrefix(content, oc+theirSep(oc)) {
			return filesTarget{}, "another torrent in " + a.client + " uses " + content
		}
	}
	local := filepath.Clean(mapLocal(content, a.pathMap))
	if filepath.Base(local) != a.name {
		// A name holding this system's own separator would point elsewhere here.
		return filesTarget{}, local + " is not named like the torrent here"
	}
	if nearLibrary(local, keep) {
		return filesTarget{}, local + " is a Library folder or holds one"
	}
	roots := []string{}
	if a.saveRoot != "" {
		roots = append(roots, a.saveRoot)
	}
	for _, m := range a.pathMap {
		if m.Ours != "" {
			roots = append(roots, m.Ours)
		}
	}
	for _, root := range roots {
		root = filepath.Clean(root)
		if local == root || !underAny(local, []string{root}) {
			continue
		}
		for _, c := range a.cats {
			if c != "" && local == filepath.Join(root, c) {
				return filesTarget{}, local + " is the label's own folder"
			}
		}
		fi, err := os.Lstat(local)
		if err != nil {
			return filesTarget{}, "HolyArrs cannot see " + local + "; they stay where " + a.client + " put them (" + content + ")"
		}
		if !fi.Mode().IsRegular() && !fi.IsDir() {
			return filesTarget{}, local + " is a link or a special file"
		}
		rel, err := filepath.Rel(root, local)
		if err != nil {
			return filesTarget{}, err.Error()
		}
		return filesTarget{root: root, rel: rel}, ""
	}
	return filesTarget{}, "they are at " + content + " on the " + a.client + " machine, outside HolyArrs's save folder and path pairs; add a path pair to reach them, or delete them by hand"
}

// nearLibrary reports whether local is in a Library folder or holds one, so
// HolyArrs never deletes it.
func nearLibrary(local string, keep []string) bool {
	for _, k := range keep {
		if k = strings.TrimSpace(k); k != "" && (underAny(local, []string{k}) || underAny(filepath.Clean(k), []string{local})) {
			return true
		}
	}
	return false
}

// remove deletes the target without leaving its root. It says why on failure.
func (t filesTarget) remove() string {
	if t.root == "" || t.rel == "" || t.rel == "." {
		return "nothing to delete"
	}
	root, err := os.OpenRoot(t.root)
	if err != nil {
		return err.Error()
	}
	defer root.Close()
	if err := root.RemoveAll(t.rel); err != nil {
		return err.Error()
	}
	return ""
}

// theirSep is the separator of a path on the Downloader's machine: a
// backslash only for a path that holds backslashes and no slash, as a Windows
// Downloader reports it.
func theirSep(p string) string {
	if strings.Contains(p, `\`) && !strings.Contains(p, "/") {
		return `\`
	}
	return "/"
}

// theirBase is the last element of a path on the Downloader's machine.
func theirBase(p string) string {
	if theirSep(p) == "/" {
		return path.Base(p)
	}
	t := strings.TrimRight(p, `\`)
	if t == "" {
		return `\`
	}
	return t[strings.LastIndex(t, `\`)+1:]
}
