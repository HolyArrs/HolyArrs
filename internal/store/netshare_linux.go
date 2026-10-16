//go:build linux

package store

import "syscall"

// Filesystem magic numbers of network filesystems (statfs f_type).
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
}

func isNetworkShare(dir string) (bool, string, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return false, "", err
	}
	kind, ok := networkFS[int64(st.Type)&0xffffffff]
	return ok, kind, nil
}
