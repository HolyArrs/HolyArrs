//go:build lab && !linux

package main

import "os"

// The lab tests run in a Linux container; elsewhere link counts are unknown.
func labLinks(os.FileInfo) uint64 { return 0 }
func labInode(os.FileInfo) uint64 { return 0 }
