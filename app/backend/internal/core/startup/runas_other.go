//go:build !linux

package runas

import "os"

// Start applies FILE__ only: the user switch and UMASK are for Linux
// containers, so portable Windows and macOS builds keep their user.
func Start(configDir string) error {
	return LoadFileEnv(os.Environ(), os.ReadFile, os.Setenv)
}
