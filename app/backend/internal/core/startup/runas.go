package startup

import (
	"errors"
	"fmt"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/files"
	"io/fs"
	"path"
	"strconv"
	"strings"
	"syscall"
)

// Marker is set in the environment of the process started after the switch,
// so it never tries to switch again.
const Marker = "HOLYARRS_USER_SWITCHED"

// filePrefix starts every variable whose value is read from a file.
const filePrefix = "FILE__"

// defaultID is the user and group HolyArrs switches to without PUID/PGID.
const defaultID = 1000

const (
	// Keep runs as the current user (not started as root).
	Keep RunAsAction = iota
	// Switch fixes the config folder's owner, switches to UID:GID and
	// starts again.
	Switch
	// StayRoot keeps running as root because PUID=0 and PGID=0 asked for it.
	StayRoot
)

// NotInContainer is the start-up error for root outside a container without
// PUID/PGID, where switching to 1000:1000 could hand the folder to a stranger.
const NotInContainer = "started as root outside a container: set PUID and PGID, or run HolyArrs as its own service user"

// nobody is the user ID a file shows when its owner is not mapped in this
// user namespace.
const nobody = 65534

// startMask is the umask HolyArrs runs with. Linux sets it at start-up; other
// systems keep the usual 022.
var startMask = 0o022

// ownFiles are HolyArrs's own files in the config folder (the databases with
// their -wal and -shm, the master key, the logs). Files copied in by root
// (docker cp into a named volume, sudo cp, a root backup tool) keep root as
// owner while the folder itself is right, so these are checked too.
var ownFiles = []string{
	"holyarrs.db", "holyarrs.db-wal", "holyarrs.db-shm",
	"holyarrs-events.db", "holyarrs-events.db-wal", "holyarrs-events.db-shm",
	"holyarrs.key", "logs", "logs/holyarrs.log",
}

// RunAsAction is what start-up does about the user.
type RunAsAction int

// RunAsPlan is what DecideRunAs chose.
type RunAsPlan struct {
	Action   RunAsAction
	UID, GID int
	// Umask is the mask to apply, or -1 to keep the current one.
	Umask int
	// Note is one line to log, or empty.
	Note string
}

// LoadFileEnv sets NAME from the file named by every FILE__NAME variable in
// environ, with one trailing newline removed. Errors name the variable and the
// file, never the value.
func LoadFileEnv(environ []string, readFile func(string) ([]byte, error), setenv func(string, string) error) error {
	for _, kv := range environ {
		key, path, ok := strings.Cut(kv, "=")
		if !ok || !strings.HasPrefix(key, filePrefix) {
			continue
		}
		name := strings.TrimPrefix(key, filePrefix)
		if name == "" {
			return fmt.Errorf("%s has no variable name after %s", key, filePrefix)
		}
		b, err := readFile(path)
		if err != nil {
			return fmt.Errorf("%s: cannot read the file %q: %w", key, path, unwrapPath(err))
		}
		v := string(b)
		if strings.HasSuffix(v, "\r\n") {
			v = v[:len(v)-2]
		} else {
			v = strings.TrimSuffix(v, "\n")
		}
		if err := setenv(name, v); err != nil {
			return fmt.Errorf("%s: cannot set %s: %w", key, name, err)
		}
	}
	return nil
}

// ParseUmask reads UMASK as an octal number such as "022" or "0002".
func ParseUmask(s string) (int, error) {
	s = strings.TrimSpace(s)
	n, err := strconv.ParseUint(s, 8, 32)
	if err != nil || n > 0o777 {
		return 0, fmt.Errorf("UMASK %q is not an octal mask such as 022 or 002", s)
	}
	return int(n), nil
}

// InContainer reports whether HolyArrs runs in a container: /.dockerenv
// (Docker), /run/.containerenv (Podman) or a non-empty container variable
// (Podman, systemd-nspawn, LXC, and this image on every runtime), or a
// Kubernetes pod.
func InContainer(exists func(string) bool, getenv func(string) string) bool {
	if getenv("container") != "" || getenv("KUBERNETES_SERVICE_HOST") != "" {
		return true
	}
	return exists("/.dockerenv") || exists("/run/.containerenv")
}

// DecideRunAs works out the plan from the effective user and group and the
// environment. switched is true in the process started after the switch.
// Outside a container root only switches when PUID or PGID is set.
func DecideRunAs(euid, egid int, lookup func(string) (string, bool), switched, inContainer bool) (RunAsPlan, error) {
	p := RunAsPlan{UID: defaultID, GID: defaultID, Umask: -1}
	if v, ok := lookup("UMASK"); ok && strings.TrimSpace(v) != "" {
		m, err := ParseUmask(v)
		if err != nil {
			return p, err
		}
		p.Umask = m
	}
	uidSet, gidSet := false, false
	if v, ok := lookup("PUID"); ok && strings.TrimSpace(v) != "" {
		n, err := parseID("PUID", v)
		if err != nil {
			return p, err
		}
		p.UID, uidSet = n, true
	}
	if v, ok := lookup("PGID"); ok && strings.TrimSpace(v) != "" {
		n, err := parseID("PGID", v)
		if err != nil {
			return p, err
		}
		p.GID, gidSet = n, true
	}
	if euid != 0 {
		p.Action = Keep
		if (uidSet || gidSet) && !switched {
			p.Note = fmt.Sprintf("PUID/PGID are ignored: the container already runs as %d:%d (its user setting)", euid, egid)
		}
		return p, nil
	}
	if uidSet && p.UID == 0 {
		if !gidSet || p.GID != 0 {
			return p, errors.New("PUID=0 needs PGID=0 to run as root; set both, or use a non-root PUID such as 1000")
		}
		p.Action = StayRoot
		p.Note = "PUID=0 and PGID=0: running as root, as asked (not recommended; PUID/PGID such as 1000 are safer)"
		return p, nil
	}
	if switched {
		return p, fmt.Errorf("still root after switching to %d:%d; HolyArrs will not run as root", p.UID, p.GID)
	}
	if !inContainer && !uidSet && !gidSet {
		return p, errors.New(NotInContainer)
	}
	p.Action = Switch
	return p, nil
}

// Perm is perm with the start-up umask applied, which is what creating a file
// with perm gives. Use it where a mode is set with chmod, so UMASK decides.
func Perm(perm fs.FileMode) fs.FileMode {
	return perm &^ fs.FileMode(startMask)
}

// NeedsOwnerFix reports whether the config folder's owner differs from the
// user HolyArrs switches to. Only then is the folder walked, so a normal start
// costs one stat.
func NeedsOwnerFix(curUID, curGID, uid, gid int) bool {
	return curUID != uid || curGID != gid
}

// Groups is the group list after the switch: gid first, then the
// supplementary groups the process already has (Docker's group_add), without
// duplicates and without root's group 0 unless gid is 0.
func Groups(gid int, current []int) []int {
	out := []int{gid}
	for _, g := range current {
		if g == 0 && gid != 0 {
			continue
		}
		dup := false
		for _, o := range out {
			dup = dup || o == g
		}
		if !dup {
			out = append(out, g)
		}
	}
	return out
}

// MountsUnder lists the mount points in mountinfo (the text of
// /proc/self/mountinfo) that lie strictly inside dir, as paths relative to
// dir. dir must be absolute and free of symlinks. Mount points equal to dir or
// outside it are left out.
func MountsUnder(mountinfo, dir string) map[string]bool {
	dir = path.Clean(dir)
	prefix := dir
	if prefix != "/" {
		prefix += "/"
	}
	out := map[string]bool{}
	for _, line := range strings.Split(mountinfo, "\n") {
		f := strings.Fields(line)
		if len(f) < 5 {
			continue
		}
		mp := path.Clean(unescapeMount(f[4]))
		if mp == dir || !strings.HasPrefix(mp, prefix) {
			continue
		}
		out[mp[len(prefix):]] = true
	}
	return out
}

// SwitchFailed is the start-up error when root cannot switch user. EINVAL
// means the ID does not exist in this user namespace (unprivileged LXC,
// rootless Docker/Podman); anything else is most likely missing
// capabilities.
func SwitchFailed(uid, gid int, err error) error {
	if errors.Is(err, syscall.EINVAL) {
		return fmt.Errorf("started as root but could not switch to %d:%d (%v): PUID/PGID %d:%d does not exist inside this container "+
			"(most likely unprivileged LXC or rootless Docker/Podman). Pick IDs this container maps, "+
			"or see the Rootless Docker and Podman section of the install guide. HolyArrs will not run as root",
			uid, gid, err, uid, gid)
	}
	return fmt.Errorf("started as root but could not switch to %d:%d (%v). "+
		"Add back the CHOWN, DAC_OVERRIDE, SETUID and SETGID capabilities if the container drops them (cap_drop: ALL), "+
		"or run the container as that user with user: \"%d:%d\". HolyArrs will not run as root",
		uid, gid, err, uid, gid)
}

// OwnFilesNeedFix reports whether any of HolyArrs's own files that exists is
// not owned by uid:gid. owner returns ok false for a file that is not there.
// A normal start costs a few stats.
func OwnFilesNeedFix(owner func(name string) (uid, gid int, ok bool), uid, gid int) bool {
	for _, name := range ownFiles {
		if u, g, ok := owner(name); ok && NeedsOwnerFix(u, g, uid, gid) {
			return true
		}
	}
	return false
}

// unwrapPath drops the path from a file error, since the message already
// names it.
func unwrapPath(err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}

func parseID(name, s string) (int, error) {
	n, err := strconv.ParseUint(strings.TrimSpace(s), 10, 32)
	if err != nil || n == 1<<32-1 {
		return 0, fmt.Errorf("%s %q is not a number such as 1000", name, s)
	}
	return int(n), nil
}

// ownerWarning is the log line when the config folder could not be given
// to uid:gid. err is the first failure, owner the folder's owner as seen
// here, uidMap and gidMap this process's uid_map and gid_map, and selinux
// its SELinux type ("" when off).
func ownerWarning(dir string, uid, gid int, err error, owner int, uidMap, gidMap, selinux string) string {
	switch {
	case errors.Is(err, syscall.EROFS):
		return fmt.Sprintf("config folder %s is mounted read-only (%v); remove :ro from the /config volume, HolyArrs needs to write there", dir, err)
	case errors.Is(err, syscall.EINVAL):
		return fmt.Sprintf("could not give %s to %d:%d (%v): PUID/PGID %d:%d is not mapped inside this container (rootless Docker/Podman or unprivileged LXC); "+
			"pick IDs this container maps, or see the Rootless Docker and Podman section of the install guide", dir, uid, gid, err, uid, gid)
	case errors.Is(err, syscall.EPERM) && owner == nobody:
		// Files owned by a host user outside this container's ID range
		// show as nobody; the host-side numbers only make sense under a
		// shifted map.
		hu, ok1 := files.HostID(uidMap, uid)
		hg, ok2 := files.HostID(gidMap, gid)
		if ok1 && ok2 && (hu != uid || hg != gid) {
			return fmt.Sprintf("could not give %s to %d:%d (%v): its files belong to a host user this container can't see (rootless Docker/Podman or unprivileged LXC). "+
				"On the host, run chown -R %d:%d on the folder mounted at %s", dir, uid, gid, err, hu, hg, dir)
		}
	}
	msg := fmt.Sprintf("could not give everything in %s to %d:%d (%v). If HolyArrs cannot write there, set PUID/PGID to the folder's owner "+
		"or fix the owner on the host (with rootless Podman: podman unshare chown -R %d:%d on the host folder)", dir, uid, gid, err, uid, gid)
	if selinux != "" {
		msg += "; with SELinux on, add :Z to the /config volume"
	}
	return msg
}

// unescapeMount turns the octal escapes mountinfo uses (\040 for a space,
// \011 tab, \012 newline, \134 backslash) back into characters.
func unescapeMount(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+4 <= len(s) {
			if n, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// ownerOnFolder is the owner lookup for OwnFilesNeedFix. Like the walk, it
// leaves out mount points inside the folder (skip) and files on another
// filesystem than the folder (dev): the walk never changes those, so they
// must not start it either, or every start would walk all of /config.
func ownerOnFolder(stat func(name string) (uid, gid int, dev uint64, ok bool), dev uint64, skip map[string]bool) func(string) (int, int, bool) {
	return func(name string) (int, int, bool) {
		if skip[name] {
			return 0, 0, false
		}
		u, g, d, ok := stat(name)
		if !ok || d != dev {
			return 0, 0, false
		}
		return u, g, true
	}
}

// fixWarning is the warning after a failed owner fix. When the folder itself
// already belonged to uid:gid (topRight), only some of HolyArrs's own files
// were wrong, so the advice names those files, not the folder's owner.
func fixWarning(dir string, uid, gid int, err error, owner int, topRight bool, uidMap, gidMap, selinux string) string {
	if !topRight || errors.Is(err, syscall.EROFS) {
		return ownerWarning(dir, uid, gid, err, owner, uidMap, gidMap, selinux)
	}
	msg := fmt.Sprintf("some of HolyArrs's files in %s belong to another user and could not be given to %d:%d (%v). "+
		"If HolyArrs cannot write them, fix their owner on the NAS or host (a share with root_squash or SMB refuses it from here)", dir, uid, gid, err)
	if selinux != "" {
		msg += "; with SELinux on, add :Z to the /config volume"
	}
	return msg
}
