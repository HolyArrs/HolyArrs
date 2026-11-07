//go:build linux

package startup

import (
	"errors"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/files"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"syscall"
)

// StartRunAs applies FILE__, UMASK and PUID/PGID. Started as root it fixes the
// owner of configDir, switches user and starts the program again, so it only
// returns in the final process. Any error should stop start-up.
func StartRunAs(configDir string) error {
	switched := os.Getenv(Marker) == "1"
	_ = os.Unsetenv(Marker)
	if !switched {
		// The first process already set these before the switch, and the
		// files may only be readable by root.
		if err := LoadFileEnv(os.Environ(), os.ReadFile, os.Setenv); err != nil {
			return err
		}
	}
	exists := func(p string) bool { _, err := os.Stat(p); return err == nil }
	p, err := DecideRunAs(os.Geteuid(), os.Getegid(), os.LookupEnv, switched, InContainer(exists, os.Getenv))
	if err != nil {
		return err
	}
	if p.Umask >= 0 {
		syscall.Umask(p.Umask)
	}
	if p.Note != "" {
		log.Printf("holyarrs: %s", p.Note)
	}
	if p.Action == Switch {
		fixOwner(configDir, p.UID, p.GID)
		if err := switchUser(p.UID, p.GID); err != nil {
			return SwitchFailed(p.UID, p.GID, err)
		}
		env := append(os.Environ(), Marker+"=1")
		err := syscall.Exec("/proc/self/exe", os.Args, env)
		// Only reached when the restart failed; the switch already took,
		// so carrying on here is safe.
		log.Printf("holyarrs: could not restart as %d:%d (%v); carrying on in this process", p.UID, p.GID, err)
	}
	mask := syscall.Umask(0)
	syscall.Umask(mask)
	startMask = mask
	log.Printf("holyarrs: running as %d:%d, umask %04o", os.Geteuid(), os.Getegid(), mask)
	return nil
}

// switchUser sets the groups, then the group, then the user. With CGO off Go
// applies each to every thread.
func switchUser(uid, gid int) error {
	cur, err := syscall.Getgroups()
	if err != nil {
		return err
	}
	if err := syscall.Setgroups(Groups(gid, cur)); err != nil {
		return err
	}
	if err := syscall.Setgid(gid); err != nil {
		return err
	}
	if err := syscall.Setuid(uid); err != nil {
		return err
	}
	if os.Geteuid() == 0 {
		return errors.New("the user did not change")
	}
	return nil
}

// fixOwner gives configDir and everything in it to uid:gid when the folder's
// own owner differs, or one of HolyArrs's own files in it (OwnFilesNeedFix);
// when all match nothing is walked. The walk goes
// through os.Root, so no path can step out of the folder, even if a folder in
// it is swapped for a symlink during the walk. Other mounts inside the folder
// are left alone, like find -xdev. The folder itself is changed last, so an
// interrupted pass runs again on the next start. Mount points listed in
// /proc/self/mountinfo are skipped too, so a bind mount from the same
// filesystem is not changed. Failures (NFS root_squash, SMB, read-only
// mounts) are a warning only.
func fixOwner(configDir string, uid, gid int) {
	warn := func(err error) { log.Printf("holyarrs: warning: config folder %s: %v", configDir, err) }
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		warn(err)
		return
	}
	r, err := os.OpenRoot(configDir)
	if err != nil {
		warn(err)
		return
	}
	defer r.Close()
	top, err := r.Open(".")
	if err != nil {
		warn(err)
		return
	}
	defer top.Close()
	fi, err := top.Stat()
	if err != nil {
		warn(err)
		return
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return
	}
	dev := uint64(st.Dev)
	skip := mountsInside(configDir)
	owner := ownerOnFolder(func(name string) (int, int, uint64, bool) {
		info, err := r.Lstat(name)
		if err != nil {
			return 0, 0, 0, false
		}
		s, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return 0, 0, 0, false
		}
		return int(s.Uid), int(s.Gid), uint64(s.Dev), true
	}, dev, skip)
	topRight := !NeedsOwnerFix(int(st.Uid), int(st.Gid), uid, gid)
	if topRight && !OwnFilesNeedFix(owner, uid, gid) {
		return
	}
	log.Printf("holyarrs: giving %s to %d:%d (first start, new PUID/PGID, or files copied in by another user)", configDir, uid, gid)
	changed, failed := 0, 0
	var first error
	fail := func(err error) {
		failed++
		if first == nil {
			first = err
		}
	}
	_ = fs.WalkDir(r.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			fail(err)
			if changed == 0 {
				return fs.SkipAll
			}
			return nil
		}
		if p == "." {
			return nil
		}
		if skip[p] {
			// A mount inside the folder, even a bind mount from the same
			// filesystem (-v /srv/media:/config/media): never touched.
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		info, err := r.Lstat(p)
		if err != nil {
			return nil
		}
		s, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return nil
		}
		if uint64(s.Dev) != dev {
			// Another filesystem (a NAS share, media, downloads), also when
			// mountinfo could not be read: never touched.
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !NeedsOwnerFix(int(s.Uid), int(s.Gid), uid, gid) {
			return nil
		}
		if err := r.Lchown(p, uid, gid); err != nil {
			fail(err)
			// Nothing changed yet: the whole mount most likely refuses it.
			if changed == 0 {
				return fs.SkipAll
			}
			return nil
		}
		changed++
		return nil
	})
	if failed == 0 || changed > 0 {
		if err := top.Chown(uid, gid); err != nil {
			fail(err)
		}
	}
	if failed > 0 {
		uidMap, _ := os.ReadFile("/proc/self/uid_map")
		gidMap, _ := os.ReadFile("/proc/self/gid_map")
		attr, _ := os.ReadFile("/proc/self/attr/current")
		log.Printf("holyarrs: warning: %s", fixWarning(configDir, uid, gid, first, int(st.Uid), topRight, string(uidMap), string(gidMap), files.SELinuxType(string(attr))))
	}
}

// mountsInside reads /proc/self/mountinfo once and returns the mount points
// inside configDir, relative to it. When the folder or mountinfo cannot be
// read it returns nothing, and the walk falls back to the device check.
func mountsInside(configDir string) map[string]bool {
	// Abs first, then EvalSymlinks: a relative -config under a symlinked
	// working folder must still resolve to the path mountinfo lists.
	dir, err := filepath.Abs(configDir)
	if err != nil {
		return nil
	}
	if dir, err = filepath.EvalSymlinks(dir); err != nil {
		return nil
	}
	b, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return nil
	}
	return MountsUnder(string(b), dir)
}
