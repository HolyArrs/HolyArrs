//go:build linux

package plugins

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// folderID is a folder's device and inode number.
func folderID(path string) (dev, ino uint64, err error) {
	var st unix.Stat_t
	if err := unix.Stat(path, &st); err != nil {
		return 0, 0, err
	}
	return uint64(st.Dev), st.Ino, nil
}

// openFolder opens an approved folder without following any link on the
// way (openat2 RESOLVE_NO_SYMLINKS; older kernels check the path resolves
// to itself) and checks it is still the same folder.
func openFolder(path string, dev, ino uint64) (*os.File, error) {
	how := unix.OpenHow{Flags: unix.O_PATH | unix.O_DIRECTORY | unix.O_CLOEXEC, Resolve: unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS}
	fd, err := unix.Openat2(unix.AT_FDCWD, path, &how)
	if errors.Is(err, unix.ENOSYS) {
		real, rerr := filepath.EvalSymlinks(path)
		if rerr != nil {
			return nil, rerr
		}
		if real != path {
			return nil, fmt.Errorf("%s now leads to %s", path, real)
		}
		fd, err = unix.Open(path, unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	}
	if err != nil {
		return nil, err
	}
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		unix.Close(fd)
		return nil, err
	}
	if uint64(st.Dev) != dev || st.Ino != ino {
		unix.Close(fd)
		return nil, fmt.Errorf("%s is another folder now", path)
	}
	return os.NewFile(uintptr(fd), path), nil
}
