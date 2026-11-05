//go:build !unix

package doctor

import "os"

// ownerOf has no uid:gid outside Unix.
func ownerOf(os.FileInfo) string { return "" }
