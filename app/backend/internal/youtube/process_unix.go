//go:build !windows

package youtube

import (
	"os/exec"
	"syscall"
)

// startOwnGroup runs cmd in its own process group, so stopping it also
// stops the ffmpeg it started.
func startOwnGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killGroup(cmd *exec.Cmd) error {
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
