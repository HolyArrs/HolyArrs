//go:build !linux

package plugins

// NetLockReason is why the network lock is missing: plugins run only on Linux.
func NetLockReason() string { return FileLockOldKernel }
