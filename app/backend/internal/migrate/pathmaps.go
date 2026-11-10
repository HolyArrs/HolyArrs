package migrate

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Path rewrites from Sonarr, Radarr and Lidarr (b2-1, decision 32): each
// one says where a download client on a host keeps a folder, and where the
// app sees that same folder. They move onto the Downloader at that host as
// its folder maps.

// maxPathMaps is the most folder maps one Downloader gets from a move.
const maxPathMaps = 100

// pathRow is one usable path rewrite read from one app.
type pathRow struct {
	host, theirs, ours, from string
}

// mapSide cleans one side of a path rewrite: "" unless it is absolute (a
// "/" path or a drive letter) with no ".." part. Its separators are kept,
// trailing ones dropped.
func mapSide(p string) string {
	p = clip(p, maxPath)
	abs := strings.HasPrefix(p, "/") ||
		len(p) >= 3 && p[1] == ':' && (p[2] == '/' || p[2] == '\\') && (p[0] >= 'a' && p[0] <= 'z' || p[0] >= 'A' && p[0] <= 'Z')
	if !abs {
		return ""
	}
	for _, part := range strings.FieldsFunc(p, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == ".." {
			return ""
		}
	}
	t := strings.TrimRight(p, `/\`)
	switch {
	case t == "":
		return "/"
	case len(t) == 2 && t[1] == ':':
		return p[:3]
	}
	return t
}

// pathMappings reads one app's path rewrites; the usable ones wait for
// pathMaps, once every app is read.
func (b *builder) pathMappings(s *Snapshot) {
	b.cover("remote_path_mappings")
	for i, m := range s.RemotePathMappings {
		from := b.from("remote_path_mappings", i)
		host := strings.ToLower(clip(m.Host, 200))
		theirs, ours := mapSide(m.RemotePath), mapSide(m.LocalPath)
		what := clip(m.RemotePath, 200) + " -> " + clip(m.LocalPath, 200)
		if host == "" || theirs == "" || ours == "" {
			b.not(GroupDownloaders, from, what, "a folder map needs a host and two full paths without ..")
			continue
		}
		// The app's side is the old app's own view; the owner's path rules
		// turn it into HolyArrs's view, as for library folders.
		if np, ok := MapPath(b.rules, ours); ok && np != "" {
			ours = np
		}
		b.paths = append(b.paths, pathRow{host: host, theirs: theirs, ours: ours, from: from})
	}
}

// pathMaps puts the path rewrites on the planned Downloaders by host. Two
// apps that send the same client folder to different places clash, and
// neither pair is written. The longest client folder comes first, so the
// most precise map wins.
func (b *builder) pathMaps() {
	key := func(r pathRow) string { return r.host + "\x00" + r.theirs }
	ours := map[string]string{}
	clash := map[string]bool{}
	for _, r := range b.paths {
		if o, ok := ours[key(r)]; ok && o != r.ours {
			clash[key(r)] = true
		}
		ours[key(r)] = r.ours
	}
	placed, capped := map[string]bool{}, map[string]bool{}
	g := b.items[GroupDownloaders]
	for i := range g.Items {
		d := g.Items[i].Downloader
		if d == nil {
			continue
		}
		u, err := url.Parse(d.URL)
		if err != nil {
			continue
		}
		host := strings.ToLower(u.Hostname())
		var maps []domain.PathMap
		seen := map[string]bool{}
		for _, r := range b.paths {
			if r.host != host || clash[key(r)] {
				continue
			}
			placed[r.from] = true
			if !seen[r.theirs] {
				seen[r.theirs] = true
				maps = append(maps, domain.PathMap{Theirs: r.theirs, Ours: r.ours})
			}
		}
		sort.SliceStable(maps, func(a, b int) bool { return len(maps[a].Theirs) > len(maps[b].Theirs) })
		if len(maps) > maxPathMaps {
			for _, m := range maps[maxPathMaps:] {
				for _, r := range b.paths {
					if r.host == host && r.theirs == m.Theirs {
						capped[r.from] = true
						b.not(GroupDownloaders, r.from, m.Theirs+" -> "+m.Ours, fmt.Sprintf("more than %d folder maps for one Downloader; add it yourself", maxPathMaps))
					}
				}
			}
			maps = maps[:maxPathMaps]
		}
		if len(maps) > 0 {
			d.PathMap = maps
			g.Items[i].Detail += fmt.Sprintf("; %d folder map(s) from the old apps", len(maps))
		}
	}
	for _, r := range b.paths {
		switch {
		case capped[r.from]:
			// Listed with the cap above.
		case placed[r.from]:
			b.p.Mapped = append(b.p.Mapped, r.from)
		case clash[key(r)]:
			b.not(GroupDownloaders, r.from, r.theirs+" -> "+r.ours, "another app sends this client folder somewhere else; neither is imported, add the right one yourself")
		default:
			b.not(GroupDownloaders, r.from, r.theirs+" -> "+r.ours, "no Downloader at "+r.host+" is imported")
		}
	}
}

// inConfigFolder says whether p is HolyArrs's own config folder or inside
// it: a folder map must never lead there.
func inConfigFolder(p, dir string) bool {
	if dir == "" {
		return false
	}
	p, dir = filepath.Clean(filepath.FromSlash(p)), filepath.Clean(dir)
	if runtime.GOOS == "windows" {
		p, dir = strings.ToLower(p), strings.ToLower(dir)
	}
	return p == dir || strings.HasPrefix(p, strings.TrimRight(dir, `/\`)+string(filepath.Separator))
}

// safeMaps drops the folder maps that lead into the config folder and says
// which were dropped.
func safeMaps(maps []domain.PathMap, dir string) (keep []domain.PathMap, dropped []string) {
	keep = []domain.PathMap{}
	for _, m := range maps {
		if inConfigFolder(m.Ours, dir) {
			dropped = append(dropped, m.Theirs)
			continue
		}
		keep = append(keep, m)
	}
	return keep, dropped
}

// unseenMaps lists the folder maps whose HolyArrs folder is not there (or
// is not a folder) on this server: often a mount the old apps had and
// HolyArrs does not have yet. They are still written.
func unseenMaps(maps []domain.PathMap) string {
	var out []string
	for _, m := range maps {
		if fi, err := os.Stat(filepath.FromSlash(m.Ours)); err != nil || !fi.IsDir() {
			out = append(out, m.Ours)
		}
	}
	return clip(strings.Join(out, ", "), 500)
}

// mapList says folder maps in a few words for a report line.
func mapList(maps []domain.PathMap) string {
	parts := make([]string, 0, len(maps))
	for _, m := range maps {
		parts = append(parts, m.Theirs+" -> "+m.Ours)
	}
	return clip(strings.Join(parts, ", "), 1000)
}
