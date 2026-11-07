//go:build !linux

package store

// isNetworkShare has no check outside Linux yet; the container runs on Linux.
func isNetworkShare(string) (bool, string, error) { return false, "", nil }
