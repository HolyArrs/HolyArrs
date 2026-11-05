//go:build linux

package runas

import (
	"errors"
	"io/fs"
	"log"
	"os"
	"syscall"
)

// Start applies FILE__, UMASK and PUID/PGID. Started as root it fixes the
// owner of configDir, switches user and starts the program again, so it only
// returns in the final process. Any error should stop start-up.
func Start(configDir string) error {
	switched := os.Getenv(Marker) == "1"
	_ = os.Unsetenv(Marker)
	if !switched {
		// The first process already set these before the switch, and the
		// files may only be readable by root.
		if err := LoadFileEnv(os.Environ(), os.ReadFile, os.Setenv); err != nil {
			return err
		}
	}
	p, err := Decide(os.Geteuid(), os.Getegid(), os.LookupEnv, switched)
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
// own owner differs; when it already matches nothing is walked. The walk goes
// through os.Root, so no path can step out of the folder, even if a folder in
// it is swapped for a symlink during the walk. Other mounts inside the folder
// are left alone, like find -xdev. The folder itself is changed last, so an
// interrupted pass runs again on the next start. Failures (NFS root_squash,
// SMB, read-only mounts) are a warning only.
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
	if !ok || !NeedsOwnerFix(int(st.Uid), int(st.Gid), uid, gid) {
		return
	}
	dev := st.Dev
	log.Printf("holyarrs: giving %s to %d:%d (first start or new PUID/PGID)", configDir, uid, gid)
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
		info, err := r.Lstat(p)
		if err != nil {
			return nil
		}
		s, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return nil
		}
		if s.Dev != dev {
			// Another mount (a NAS share, media, downloads): never touched.
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
		log.Printf("holyarrs: warning: could not give everything in %s to %d:%d (%v). "+
			"This is normal on NFS with root_squash, SMB and read-only mounts; if HolyArrs cannot write there, "+
			"set PUID/PGID to the folder's owner or fix the owner on the host", configDir, uid, gid, first)
	}
}
