package files

import (
	"errors"
	"path"
	"strconv"
	"strings"
	"syscall"
)

const desktopFix = "use a named volume for /config (-v holyarrs-config:/config); media folders can stay as they are"

// mount is one mountinfo line: the root inside its filesystem, where it is
// mounted, and the filesystem type.
type mount struct{ root, point, fstype string }

// FSType is the filesystem type of the mount that holds dir ("ext4",
// "9p", "fuse.shfs"), or "" when unsure. dir should already be absolute
// with symlinks resolved. Only the type is returned, limited to plain
// characters.
func FSType(mountinfo, dir string) string {
	m, ok := best(mountinfo, dir)
	if !ok || !plainType(m.fstype) {
		return ""
	}
	return m.fstype
}

// Unescape turns the octal escapes mountinfo uses (\040 for a space, \011
// tab, \012 newline, \134 backslash) back into characters.
func Unescape(s string) string {
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

// Family names a filesystem the database cannot use safely in everyday
// words, with the fix. A local disk (and anything not listed) gives "", "".
func Family(fstype string) (label, fix string) {
	switch fstype {
	case "9p", "fuse.grpcfuse", "osxfs", "fakeowner":
		return "a folder shared from the host (Docker Desktop or a VM folder share)", desktopFix
	case "fuse.shfs":
		return "an Unraid user share (/mnt/user)", "use /mnt/<pool>/appdata/holyarrs, or set the appdata share to Exclusive access"
	case "fuse.mergerfs":
		return "a mergerfs pool", "put /config on one member disk of the pool, not on the pooled path"
	case "fuseblk", "fuse.ntfs-3g", "ntfs-3g", "ntfs", "ntfs3", "exfat", "vfat":
		return "a Windows-format disk (NTFS, exFAT or FAT)", "put /config on the system disk or on an ext4, xfs or btrfs disk"
	case "virtiofs":
		return "a VM folder share (virtiofs)", "put /config on the VM's own disk"
	case "fuse.rclone", "fuse.sshfs", "fuse.s3fs", "rclone", "sshfs", "s3fs":
		return "a remote mount (rclone, sshfs or s3fs)", "put /config on a local disk"
	case "nfs", "nfs4", "cifs", "smb", "smb2", "smb3":
		return "a network share (NFS or SMB)", "put /config on a local disk of the machine that runs HolyArrs; media folders can stay on the share"
	}
	if fstype == "fuse" || strings.HasPrefix(fstype, "fuse.") {
		return "a FUSE filesystem (for example Unraid /mnt/user, mergerfs, rclone, ntfs-3g)", "put /config on a local disk (on Unraid: /mnt/<pool>/appdata/holyarrs)"
	}
	return "", ""
}

// Why says in plain words why a write failed, or "" when the error is not
// one of the usual ones.
func Why(err error) string {
	var no syscall.Errno
	if err == nil || !errors.As(err, &no) {
		return ""
	}
	switch no {
	case syscall.EROFS:
		return "the folder is mounted read-only"
	case syscall.EACCES, syscall.EPERM:
		return "no write permission"
	case syscall.ENOSPC, syscall.EDQUOT:
		return "the disk is full or over its quota"
	case syscall.EINVAL:
		return "the owner's user ID is not mapped in this container"
	}
	return ""
}

// HostID is the host-side number of id inside a user namespace, read from
// a uid_map or gid_map (inside outside count per line). ok is false when no
// range or more than one range covers it.
func HostID(idMap string, id int) (int, bool) {
	got, n := 0, 0
	for _, line := range strings.Split(idMap, "\n") {
		f := strings.Fields(line)
		if len(f) != 3 {
			continue
		}
		in, err1 := strconv.ParseInt(f[0], 10, 64)
		out, err2 := strconv.ParseInt(f[1], 10, 64)
		cnt, err3 := strconv.ParseInt(f[2], 10, 64)
		if err1 != nil || err2 != nil || err3 != nil {
			continue
		}
		if v := int64(id); v >= in && v < in+cnt {
			got, n = int(out+v-in), n+1
		}
	}
	if n != 1 {
		return 0, false
	}
	return got, true
}

// SELinuxType is the type part of an SELinux label (/proc/self/attr/current,
// user:role:type:level), without the categories. AppArmor labels and
// anything else give "".
func SELinuxType(attr string) string {
	attr = strings.TrimSpace(strings.Trim(attr, "\x00\n"))
	if strings.ContainsAny(attr, " ()") {
		return ""
	}
	p := strings.Split(attr, ":")
	if len(p) < 4 || p[2] == "" {
		return ""
	}
	for _, r := range p[2] {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_') {
			return ""
		}
	}
	return p[2]
}

// Unmapped tells whether dir (inside a container) is a forgotten volume:
// "anonymous" for an anonymous Docker or Podman volume (it is gone once the
// container is removed), "none" when nothing is mounted there and it sits in
// the container's own layer. Anything else, or anything unsure, gives "".
func Unmapped(mountinfo, dir string) string {
	m, ok := best(mountinfo, dir)
	if !ok {
		return ""
	}
	if m.point == "/" {
		switch m.fstype {
		case "overlay", "aufs", "fuse.fuse-overlayfs":
			return "none"
		}
		return ""
	}
	p := strings.Split(strings.TrimSuffix(m.root, "/"), "/")
	if n := len(p); n >= 3 && p[n-1] == "_data" && p[n-3] == "volumes" && isHex64(p[n-2]) {
		return "anonymous"
	}
	return ""
}

// parse reads one mountinfo line (id parent major:minor root mountpoint
// options [optional...] - fstype source superoptions). The source and the
// options are never kept: they can hold host paths, share names or
// credentials.
func parse(line string) (mount, bool) {
	f := strings.Fields(line)
	if len(f) < 5 {
		return mount{}, false
	}
	for i := 5; i < len(f)-1; i++ {
		if f[i] == "-" {
			return mount{root: Unescape(f[3]), point: path.Clean(Unescape(f[4])), fstype: f[i+1]}, true
		}
	}
	return mount{}, false
}

// best is the mount that holds dir: the longest mount point that is dir or
// one of its parents. Later lines win a tie (a mount on top of another).
func best(mountinfo, dir string) (mount, bool) {
	dir = path.Clean(dir)
	var out mount
	found := false
	for _, line := range strings.Split(mountinfo, "\n") {
		m, ok := parse(line)
		if !ok {
			continue
		}
		if m.point != "/" && m.point != dir && !strings.HasPrefix(dir, m.point+"/") {
			continue
		}
		if !found || len(m.point) >= len(out.point) {
			out, found = m, true
		}
	}
	return out, found
}

func plainType(s string) bool {
	if s == "" || len(s) > 32 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

func isHex64(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}
