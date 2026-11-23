package migrate

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// systemMounts are mount roots that never hold media.
var systemMounts = []string{"/proc", "/sys", "/dev", "/etc", "/run", "/boot", "/snap", "/usr", "/var", "/tmp", "/config"}

// SeenFolders lists the folders mounted into HolyArrs (inside a container,
// the right side of each volume), so the folder step can suggest them. It
// is empty where there is no mount list.
func SeenFolders() []string {
	b, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return []string{}
	}
	return seenFolders(string(b), isDir)
}

func seenFolders(mountinfo string, dir func(string) bool) []string {
	out := []string{}
	for rel := range startup.MountsUnder(mountinfo, "/") {
		p := "/" + rel
		if dir(p) && !underAny(systemMounts, p) {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	if len(out) > 50 {
		out = out[:50]
	}
	return out
}

func underAny(roots []string, p string) bool {
	for _, r := range roots {
		if p == r || strings.HasPrefix(p, r+"/") {
			return true
		}
	}
	return false
}

// missingTargets lists, by old folder, the new folders the path rules point
// at that are not a folder on this server, the same check the dry run
// makes later.
func missingTargets(p *Plan) map[string]string {
	out := map[string]string{}
	for _, g := range p.Groups {
		for _, it := range g.Items {
			if f := it.Folder; f != nil && f.Path != f.OldPath && !isDir(filepath.FromSlash(f.Path)) {
				out[f.OldPath] = f.Path
			}
		}
	}
	return out
}

// MarkUnseen marks each old root folder this server cannot see as it is,
// so the folder step words its choices for what they do there: by default
// its titles come over as wanted and need attention, and "Don't bring them"
// keeps them behind.
func MarkUnseen(p *Plan) {
	for gi := range p.Groups {
		for i := range p.Groups[gi].Items {
			if f := p.Groups[gi].Items[i].Folder; f != nil {
				f.Unseen = !isDir(filepath.FromSlash(f.OldPath))
			}
		}
	}
}

// foundMax is how many title folders MarkFound looks for in each folder.
const foundMax = 1000

// MarkFound counts, for each Library folder, how many of its titles'
// folders are really in the folder it now points at, so a wrong pick shows
// at once. It only looks each folder up and never lists or reads anything.
func MarkFound(p *Plan, rt *rooted) {
	markFound(p, rt.dir)
	// The import writes into each folder: a folder it cannot write to is
	// told now, not as hundreds of failed titles later. The check's probe
	// file is removed again.
	for gi := range p.Groups {
		for i := range p.Groups[gi].Items {
			// Only inside the mounted and Library folders: the probe writes a file.
			if f := p.Groups[gi].Items[i].Folder; f != nil && !f.Left && !f.Missing && f.Checked > 0 && underAny(rt.roots, path.Clean(f.Path)) {
				f.ReadOnly = library.CheckFolder(filepath.FromSlash(f.Path), nil).State == "read_only"
			}
		}
	}
}

func markFound(p *Plan, dir func(string) bool) {
	// Only titles the old app had files for: one still waiting to download
	// has no folder yet. A library where no title says so counts them all.
	names, withFiles := map[string][]string{}, map[string][]string{}
	for _, g := range p.Groups {
		for _, it := range g.Items {
			if t := it.Title; t != nil && t.Unresolved == "" && t.Folder != "" && len(names[t.Library]) < foundMax {
				names[t.Library] = append(names[t.Library], t.Folder)
				if t.hasFiles {
					withFiles[t.Library] = append(withFiles[t.Library], t.Folder)
				}
			}
		}
	}
	for lib, ns := range withFiles {
		if len(ns) > 0 {
			names[lib] = ns
		}
	}
	for gi := range p.Groups {
		for i := range p.Groups[gi].Items {
			f := p.Groups[gi].Items[i].Folder
			if f == nil || f.Left {
				continue
			}
			f.Missing = !dir(filepath.FromSlash(f.Path))
			f.Checked = len(names[f.Path])
			f.Found = countIn(f.Path, names[f.Path], dir)
		}
	}
}

// suggestNames is how many title folders MarkSuggest looks for in each
// folder here; enough to tell the right folder, few enough to stay quick.
const suggestNames = 8

// MarkSuggest finds, for each old folder this server cannot see as it is,
// the folder here that holds most of its titles' folders, so setup can pick
// it on its own. Like MarkFound it only looks folders up.
func MarkSuggest(p *Plan, here []string, dir func(string) bool) { markSuggest(p, here, dir) }

func markSuggest(p *Plan, here []string, dir func(string) bool) {
	names := map[string][]string{}
	for _, g := range p.Groups {
		for _, it := range g.Items {
			if t := it.Title; t != nil && t.Unresolved == "" && t.Folder != "" && len(names[t.Library]) < suggestNames {
				names[t.Library] = append(names[t.Library], t.Folder)
			}
		}
	}
	for gi := range p.Groups {
		for i := range p.Groups[gi].Items {
			f := p.Groups[gi].Items[i].Folder
			// Only a folder nobody pointed anywhere yet: a picked one is not searched again.
			if f == nil || f.Left || !f.Unseen || f.Path != f.OldPath || len(names[f.Path]) == 0 {
				continue
			}
			best := 0
			for _, h := range here {
				n := countIn(h, names[f.Path], dir)
				if n > best {
					best, f.Suggest = n, h
				}
			}
		}
	}
}

// hereMax is how many folders the setup picker offers at most.
const hereMax = 200

// HereFolders lists the folders a moved root folder could go to: each
// Library folder and mounted folder, and the folders one level inside them
// (so /data offers /data/Movies). Setup uses it to match old folders and to
// offer real choices; at most 200.
func HereFolders(ctx context.Context, st *database.Store) []string {
	roots := SeenFolders()
	for _, s := range libraryFolderPaths(ctx, st) {
		roots = append(roots, filepath.ToSlash(s))
	}
	return hereFolders(roots, func(p string) []string {
		// Read in small batches and stop at the cap: a mount with a huge
		// number of entries stays quick and light.
		f, err := os.Open(filepath.FromSlash(p))
		if err != nil {
			return nil
		}
		defer f.Close()
		var out []string
		for len(out) < hereMax {
			es, err := f.ReadDir(256)
			for _, e := range es {
				if e.IsDir() && !strings.HasPrefix(e.Name(), ".") && len(out) < hereMax {
					out = append(out, strings.TrimSuffix(p, "/")+"/"+e.Name())
				}
			}
			if err != nil || len(es) == 0 {
				break
			}
		}
		return out
	})
}

func hereFolders(roots []string, sub func(string) []string) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(p string) {
		if !seen[p] && len(out) < hereMax {
			seen[p] = true
			out = append(out, p)
		}
	}
	for _, r := range roots {
		add(r)
		for _, s := range sub(r) {
			add(s)
		}
	}
	sort.Strings(out)
	return out
}
