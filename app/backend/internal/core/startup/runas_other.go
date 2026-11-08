//go:build !linux

package startup

import "os"

// StartRunAs applies FILE__ only: the user switch and UMASK are for Linux
// containers, so portable Windows and macOS builds keep their user.
func StartRunAs(configDir string) error {
	return LoadFileEnv(os.Environ(), os.ReadFile, os.Setenv)
}
