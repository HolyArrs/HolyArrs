//go:build unix

package health

import (
	"os"
	"strconv"
	"syscall"
)

// ownerOf is the file's uid:gid.
func ownerOf(fi os.FileInfo) string {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return ""
	}
	return strconv.FormatUint(uint64(st.Uid), 10) + ":" + strconv.FormatUint(uint64(st.Gid), 10)
}
