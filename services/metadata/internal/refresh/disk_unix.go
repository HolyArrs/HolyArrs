//go:build !windows

package refresh

import "syscall"

// DiskUsage reports used and total bytes of the volume holding path.
func DiskUsage(path string) (used, total uint64, err error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, err
	}
	total = st.Blocks * uint64(st.Bsize)
	return total - st.Bfree*uint64(st.Bsize), total, nil
}
