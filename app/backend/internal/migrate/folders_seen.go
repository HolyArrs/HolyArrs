package migrate

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
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
