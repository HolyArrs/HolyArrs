package plugins

import (
	"net/url"
	"runtime"
)

// Platforms a plugin is built for: the same CPU types as the app image.
// linux-amd64 keeps the first layout (plugins.json and
// plugins/{id}/{version}/), so apps from before per-platform builds still
// read it; every other platform has its own signed list and a folder below
// the version. All lists of one publish carry the same counter.
const (
	PlatformAMD64 = "linux-amd64"
	PlatformARM64 = "linux-arm64"
	PlatformARMv7 = "linux-arm-v7"
)

// Platforms lists every platform a plugin version is built for.
var Platforms = []string{PlatformAMD64, PlatformARM64, PlatformARMv7}

// HostPlatform is the platform of the running app. The app image is built
// for arm/v7 only, so any 32-bit arm counts as arm-v7.
func HostPlatform() string {
	switch runtime.GOARCH {
	case "amd64":
		return PlatformAMD64
	case "arm64":
		return PlatformARM64
	case "arm":
		return PlatformARMv7
	}
	return runtime.GOOS + "-" + runtime.GOARCH
}

// ListName is the signed plugin list file for platform p.
func ListName(p string) string {
	if p == PlatformAMD64 {
		return "plugins.json"
	}
	return "plugins-" + p + ".json"
}

// FileBase is the folder (with a trailing slash) holding manifest.json and
// plugin for one version on platform p.
func FileBase(id, version, p string) string {
	base := "plugins/" + url.PathEscape(id) + "/" + url.PathEscape(version) + "/"
	if p != PlatformAMD64 {
		base += url.PathEscape(p) + "/"
	}
	return base
}

// platform is the engine's platform: Platform, or the running machine's.
func (e *Engine) platform() string {
	if e.Platform != "" {
		return e.Platform
	}
	return HostPlatform()
}

// cacheListName keeps each platform's cached list apart.
func (e *Engine) cacheListName() string {
	if p := e.platform(); p != PlatformAMD64 {
		return "list-" + p + ".json"
	}
	return cacheList
}
