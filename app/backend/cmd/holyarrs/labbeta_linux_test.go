//go:build lab && linux

package main

import (
	"os"
	"syscall"
)

// labLinks is a file's hard link count.
func labLinks(fi os.FileInfo) uint64 {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return uint64(st.Nlink)
	}
	return 0
}

// labInode is a file's inode number.
func labInode(fi os.FileInfo) uint64 {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return st.Ino
	}
	return 0
}
