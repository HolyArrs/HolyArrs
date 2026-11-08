//go:build !linux

package health

// lockProbe is Linux only (the container runs on Linux); nil skips it.
var lockProbe func(string) error
