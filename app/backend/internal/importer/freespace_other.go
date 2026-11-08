//go:build !linux && !windows

package importer

// FreeSpace returns the bytes free for normal users at path, or -1 when unknown.
func FreeSpace(string) int64 { return -1 }

// DiskSize returns the total bytes of the disk holding path, or -1 when unknown.
func DiskSize(string) int64 { return -1 }

func diskID(string) string { return "" }
