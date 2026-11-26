package migrate

import (
	"context"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

const (
	// deepLevels is how far below each mount the folder search looks, so
	// /data/media/movies is found under a /data mount.
	deepLevels = 3
	// deepMax caps the folders the search looks at, so a huge mount stays quick.
	deepMax = 2000
	// browseMax is how many subfolders one browse answer lists.
	browseMax = 300
	// browseCountMax is the most folders one answer counts titles in.
	browseCountMax = 50
)

// browseRoots are the folders the setup browser starts from: the mounted
// folders and the Library folders.
func browseRoots(ctx context.Context, st *database.Store) []string {
	roots := SeenFolders()
	for _, s := range libraryFolderPaths(ctx, st) {
		roots = append(roots, filepath.ToSlash(s))
	}
	return roots
}

// rooted looks folders up only inside the given roots, through os.Root,
// so a link inside a mounted folder cannot lead the browser or the folder
// search to the rest of the host. Each root is opened once and kept until
// close.
type rooted struct {
	roots []string
	mu    sync.Mutex
	open  map[string]*os.Root
}

func newRooted(roots []string) *rooted { return &rooted{roots: roots, open: map[string]*os.Root{}} }

func (r *rooted) close() {
	for _, rt := range r.open {
		_ = rt.Close()
	}
}

// at answers the open root holding p and p's path inside it ("." for the
// root itself), or nil when p is under none of the roots.
func (r *rooted) at(p string) (*os.Root, string) {
	p = path.Clean(filepath.ToSlash(p))
	best := ""
	for _, root := range r.roots {
		root = path.Clean(filepath.ToSlash(root))
		if (p == root || strings.HasPrefix(p, strings.TrimSuffix(root, "/")+"/")) && len(root) > len(best) {
			best = root
		}
	}
	if best == "" {
		return nil, ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	rt, ok := r.open[best]
	if !ok {
		var err error
		if rt, err = os.OpenRoot(filepath.FromSlash(best)); err != nil {
			rt = nil
		}
		r.open[best] = rt
	}
	if rt == nil {
		return nil, ""
	}
	rel := strings.TrimPrefix(strings.TrimPrefix(p, best), "/")
	if rel == "" {
		rel = "."
	}
	return rt, filepath.FromSlash(rel)
}

// dir says p is a folder inside the roots.
func (r *rooted) dir(p string) bool {
	rt, rel := r.at(p)
	if rt == nil {
		return false
	}
	st, err := rt.Stat(rel)
	return err == nil && st.IsDir()
}

// sub lists the folders directly in p (inside the roots), at most max.
func (r *rooted) sub(p string, max int) []string {
	rt, rel := r.at(p)
	if rt == nil {
		return nil
	}
	f, err := rt.Open(rel)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	for len(out) < max {
		es, err := f.ReadDir(256)
		for _, e := range es {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") && len(out) < max {
				out = append(out, strings.TrimSuffix(path.Clean(p), "/")+"/"+e.Name())
			}
		}
		if err != nil || len(es) == 0 {
			break
		}
	}
	return out
}

// deepFolders lists the roots and the folders up to deepLevels below them,
// at most deepMax, for the folder search.
func deepFolders(roots []string, sub func(string) []string) []string {
	seen := map[string]bool{}
	out := []string{}
	level := roots
	for d := 0; d <= deepLevels && len(level) > 0; d++ {
		var next []string
		for _, p := range level {
			if seen[p] || len(out) >= deepMax {
				continue
			}
			seen[p] = true
			out = append(out, p)
			// A folder with many folders in it is a library: its folders are
			// titles, so the search does not open each of them.
			if d < deepLevels {
				if kids := sub(p); len(kids) <= browseCountMax {
					next = append(next, kids...)
				}
			}
		}
		level = next
	}
	return out
}

// countIn counts the names that are folders in dir, looking up to 16 at a
// time: on a network share or a slow mount each look waits on the disk.
func countIn(dir string, names []string, isFolder func(string) bool) int {
	var n atomic.Int64
	sem := make(chan struct{}, 16)
	var wg sync.WaitGroup
	for _, name := range names {
		wg.Add(1)
		sem <- struct{}{}
		go func(name string) {
			defer func() { <-sem; wg.Done() }()
			if isFolder(filepath.FromSlash(path.Join(dir, name))) {
				n.Add(1)
			}
		}(name)
	}
	wg.Wait()
	return int(n.Load())
}

// cleanNames keeps plain folder names only, at most foundMax.
func cleanNames(names []string) []string {
	out := []string{}
	for _, n := range names {
		if len(out) == foundMax {
			break
		}
		if n != "" && n != "." && n != ".." && !strings.ContainsAny(n, "/\\\x00") && len(n) <= 255 {
			out = append(out, n)
		}
	}
	return out
}

// browseFolders answers the setup folder browser: the folders in path (or
// the starting folders when path is empty), each with how many of the
// given title folders it holds. It only lists folders inside the mounted
// and Library folders, and never reads a file.
func (h *Handlers) browseFolders(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	var in struct {
		Path  string   `json:"path"`
		Names []string `json:"names"`
	}
	if !webserver.Decode(w, r, &in, "send path and names") {
		return
	}
	roots := browseRoots(r.Context(), h.Store)
	rt := newRooted(roots)
	defer rt.close()
	p := strings.TrimSpace(in.Path)
	var list []string
	if p == "" {
		// Starting points: only folders not already inside another one
		// (a Library folder under a mounted /media shows inside /media).
		for _, x := range roots {
			if !underAny(without(roots, x), x) {
				list = append(list, x)
			}
		}
	} else {
		if !path.IsAbs(p) || len(p) > maxPath {
			webserver.Fail(w, http.StatusBadRequest, "path must be a full folder path")
			return
		}
		p = path.Clean(p)
		if !underAny(roots, p) {
			webserver.Fail(w, http.StatusBadRequest, "that folder is not one HolyArrs can see")
			return
		}
		list = rt.sub(p, browseMax)
	}
	sort.Strings(list)
	names := cleanNames(in.Names)
	// A folder with many folders in it is a library already (its folders
	// are titles): counting inside each would only make it slow.
	if len(list) > browseCountMax {
		names = nil
	}
	type entry struct {
		Path  string `json:"path"`
		Found int    `json:"found"`
	}
	out := make([]entry, 0, len(list))
	for _, f := range list {
		out = append(out, entry{f, countIn(f, names, rt.dir)})
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"path": p, "folders": out, "checked": len(names)})
}

// needsSuggest says some old folder is unseen and not pointed anywhere yet,
// so the folder search (the slow part) runs only when it can help.
func needsSuggest(p *Plan) bool {
	for _, g := range p.Groups {
		for _, it := range g.Items {
			if f := it.Folder; f != nil && !f.Left && f.Unseen && f.Path == f.OldPath {
				return true
			}
		}
	}
	return false
}

// without returns list minus one value.
func without(list []string, v string) []string {
	out := make([]string, 0, len(list))
	for _, x := range list {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}
