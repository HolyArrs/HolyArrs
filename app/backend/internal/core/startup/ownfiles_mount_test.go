package startup

import (
	"strings"
	"syscall"
	"testing"
)

// An own file that is its own mount (-v /volume1/logs:/config/logs), or
// sits on another filesystem, is never changed by the walk, so it must not
// start the walk either: else every start walks all of /config for nothing.
func TestOwnFilesSkipMountsLikeTheWalk(t *testing.T) {
	stat := func(name string) (int, int, uint64, bool) {
		switch name {
		case "logs":
			return 1026, 100, 1, true // listed in mountinfo
		case "logs/holyarrs.log":
			return 1026, 100, 2, true // another device
		case "holyarrs.db":
			return 1000, 1000, 1, true
		}
		return 0, 0, 0, false
	}
	owner := ownerOnFolder(stat, 1, map[string]bool{"logs": true})
	if OwnFilesNeedFix(owner, 1000, 1000) {
		t.Error("a mounted logs folder with another owner should not start the walk")
	}
	owner = ownerOnFolder(func(name string) (int, int, uint64, bool) {
		if name == "holyarrs.key" {
			return 0, 0, 1, true
		}
		return stat(name)
	}, 1, map[string]bool{"logs": true})
	if !OwnFilesNeedFix(owner, 1000, 1000) {
		t.Error("a root-owned key on the same disk should start the walk")
	}
}

// When the folder itself already belongs to PUID/PGID and only some of
// HolyArrs's files could not be given to it (NFS root_squash, SMB), the
// warning must not tell the person to set PUID/PGID to the folder's owner.
func TestOwnFilesWarningFitsARightFolder(t *testing.T) {
	msg := fixWarning("/config", 1000, 1000, syscall.EPERM, 1000, true, "", "", "")
	if strings.Contains(msg, "folder's owner") {
		t.Errorf("advice does not fit a folder that is already right: %s", msg)
	}
	for _, want := range []string{"HolyArrs's files", "1000:1000"} {
		if !strings.Contains(msg, want) {
			t.Errorf("lacks %q: %s", want, msg)
		}
	}
	if ro := fixWarning("/config", 1000, 1000, syscall.EROFS, 1000, true, "", "", ""); !strings.Contains(ro, ":ro") {
		t.Errorf("read-only keeps its own advice: %s", ro)
	}
	if top := fixWarning("/config", 1000, 1000, syscall.EPERM, 0, false, "", "", ""); top != ownerWarning("/config", 1000, 1000, syscall.EPERM, 0, "", "", "") {
		t.Errorf("a wrong top folder keeps the old warning: %s", top)
	}
}
