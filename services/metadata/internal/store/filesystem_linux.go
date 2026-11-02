package store

import (
	"errors"
	"os"
	"syscall"
)

func lockDirectory(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, errors.New("cannot open storage ownership lock")
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, ErrOwned
	}
	return f, nil
}

func localSpace(path string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, errors.New("cannot inspect storage filesystem")
	}
	// Only known local filesystems are accepted; NFS, SMB, 9P and unknown
	// filesystem implementations cannot promise SQLite's local lock semantics.
	switch uint64(stat.Type) {
	case 0xef53, 0x58465342, 0x9123683e, 0x2fc12fc1, 0xf2f52010, 0x01021994, 0x794c7630:
		return stat.Bavail * uint64(stat.Bsize), nil
	default:
		return 0, errors.New("storage requires a supported local filesystem")
	}
}
