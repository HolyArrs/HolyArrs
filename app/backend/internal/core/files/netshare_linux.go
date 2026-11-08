//go:build linux

package files

import "syscall"

// Filesystem magic numbers (statfs f_type) of folders the database cannot
// use safely: network filesystems, host folder shares, FUSE, and memory.
var networkFS = map[int64]string{
	0x6969:     "nfs",
	0x517b:     "smb",
	0xff534d42: "cifs",
	0xfe534d42: "smb2",
	0x01021997: "9p",
	0x65735546: "fuse",
	0x5346414f: "afs",
	0x47504653: "gpfs",
	0x0bd00bd0: "lustre",
	0x73757245: "coda",
	0x564c:     "ncp",
	// Docker Desktop for Mac binds (4.74 and later), where locks are broken.
	0x6a656a63: "fakeowner",
	// Memory: everything in it is gone after a restart.
	0x01021994: "tmpfs",
	0x858458f6: "ramfs",
}

func IsNetworkShare(dir string) (bool, string, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return false, "", err
	}
	kind := fsKindOf(int64(st.Type))
	return kind != "", kind, nil
}

// fsKindOf names a statfs magic number, or "" for a local disk.
func fsKindOf(magic int64) string { return networkFS[magic&0xffffffff] }
