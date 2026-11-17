package plugins

import "runtime"

// HostPlatform is the plugin build name for this machine, for example
// "linux-amd64". 32-bit ARM images are built for ARMv7 only.
func HostPlatform() string {
	if runtime.GOARCH == "arm" {
		return runtime.GOOS + "-armv7"
	}
	return runtime.GOOS + "-" + runtime.GOARCH
}
