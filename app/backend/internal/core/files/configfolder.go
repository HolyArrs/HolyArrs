package files

import (
	"os"
	"path/filepath"
	"strings"
)

// Overrides for the refusals above. Each lifts only its own kind.
const (
	// EnvAllowDesktopFolder lifts the Docker Desktop for Mac (fakeowner)
	// refusal, for someone who checked that locks work there.
	EnvAllowDesktopFolder = "HOLYARRS_ALLOW_DESKTOP_FOLDER"
	// EnvConfigInMemory keeps the config folder in memory anyway.
	EnvConfigInMemory = "HOLYARRS_CONFIG_IN_MEMORY"
	// EnvAllowFuseFolder lifts the FUSE refusal (mergerfs and other FUSE
	// kinds), for someone who chooses to keep /config there anyway. An
	// Unraid user share (fuse.shfs) needs no override.
	EnvAllowFuseFolder = "HOLYARRS_ALLOW_FUSE_FOLDER"
)

// ShareFix is the plain fix for a config folder refused as a share of this
// kind. It holds no path, so the start-up page can show it.
func ShareFix(kind string) string {
	switch kind {
	case "9p":
		return `This folder is shared from the host (Docker Desktop on Windows shares C:\ folders this way, and so do VM folder shares). The database is not safe there. Put /config on a named volume (holyarrs-config:/config), or on the VM's own disk. Media folders can stay where they are.`
	case "fakeowner":
		return "This folder is shared from the Mac by Docker Desktop, where file locks do not work, so the database is not safe there. Put /config on a named volume (holyarrs-config:/config). Media folders can stay where they are. If you have checked that file locks work in this folder, set " + EnvAllowDesktopFolder + "=1 to start HolyArrs on it anyway; HolyArrs then warns on every start."
	case "fuse":
		return "This is a FUSE filesystem (for example mergerfs, rclone or sshfs), where the database is not safe. For mergerfs, put /config on one member disk of the pool. On Unraid, a user share under /mnt/user normally starts in a safer database mode; if it was refused, use /mnt/<pool>/appdata/holyarrs, or /mnt/disk1/appdata/holyarrs with no pool. On a Mac, use a named volume (holyarrs-config:/config). Otherwise put /config on a local disk. To keep it on this folder anyway, set " + EnvAllowFuseFolder + "=1; HolyArrs then warns on every start."
	}
	return "The database needs a local disk: put /config on a local disk of the machine that runs HolyArrs. Media folders can stay on the share."
}

// UnraidShareFix is the fix shown while /config is on an Unraid user share.
const UnraidShareFix = "for full speed, map /config to a pool path (/mnt/<pool>/appdata/holyarrs), or to one array disk with no pool (/mnt/disk1/appdata/holyarrs), or set the appdata share to Exclusive access; keep the mover off appdata while HolyArrs runs"

// UnraidUserShare reports a folder on an Unraid user share: the statfs
// kind is fuse and the mount type is fuse.shfs. Only this FUSE kind is
// adapted to; mergerfs and the rest stay refused.
func UnraidUserShare(kind, mountType string) bool {
	return kind == "fuse" && mountType == "fuse.shfs"
}

// DesktopFolderAllowed reports the Docker Desktop folder override.
func DesktopFolderAllowed() bool { return on(EnvAllowDesktopFolder) }

// FuseFolderAllowed reports the FUSE folder override.
func FuseFolderAllowed() bool { return on(EnvAllowFuseFolder) }

// ConfigInMemoryAllowed reports the in-memory override.
func ConfigInMemoryAllowed() bool { return on(EnvConfigInMemory) }

// InMemory reports whether a kind from NetworkShare is memory, not a share.
func InMemory(kind string) bool { return kind == "tmpfs" || kind == "ramfs" }

// MountType is the mount type of dir from /proc/self/mountinfo (Linux),
// for the log, doctor and the support bundle; "" when unknown.
func MountType(dir string) string {
	b, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return ""
	}
	if abs, err := filepath.Abs(dir); err == nil {
		if r, err := filepath.EvalSymlinks(abs); err == nil {
			dir = r
		}
	}
	return FSType(string(b), filepath.ToSlash(dir))
}

// on reads a yes/no override.
func on(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "yes", "true", "on":
		return true
	}
	return false
}
