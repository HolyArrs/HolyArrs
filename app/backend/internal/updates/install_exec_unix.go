//go:build unix

package updates

import (
	"os"
	"syscall"
)

// execSupported says this system can switch programs in place.
const execSupported = true

// execProgram replaces this process with path. The PID stays the same, so a
// container whose first process is HolyArrs never sees it stop.
func execProgram(path string, argv, env []string) error { return syscall.Exec(path, argv, env) }

// Reexec starts the image's program again in this process, so its launcher
// picks the version to run. It returns only on failure.
func Reexec() error {
	bin := os.Getenv(envImageBin)
	if bin == "" {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		bin = exe
	}
	return execProgram(bin, append([]string{bin}, os.Args[1:]...), withoutEnv(os.Environ(), envFromVolume, envImageBin, envImageVersion))
}
