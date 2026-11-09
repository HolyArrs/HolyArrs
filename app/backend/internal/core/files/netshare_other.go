//go:build !linux

package files

// IsNetworkShare has no check outside Linux yet; the container runs on Linux.
func IsNetworkShare(string) (bool, string, error) { return false, "", nil }
