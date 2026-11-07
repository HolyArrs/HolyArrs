//go:build !linux && !windows

package importer

// FreeSpace is unknown (-1) here; HolyArrs runs on Linux and Windows.
func FreeSpace(string) int64 { return -1 }

// DiskSize is unknown (-1) here.
func DiskSize(string) int64 { return -1 }

// diskID is unknown ("") here.
func diskID(string) string { return "" }
