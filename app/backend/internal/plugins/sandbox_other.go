//go:build !linux

package plugins

import "os/exec"

// RunSandboxHelper does nothing outside Linux.
func RunSandboxHelper() {}

func killProcess(cmd *exec.Cmd) error { return cmd.Process.Kill() }

// DefaultRunner refuses: plugins run only on Linux.
func DefaultRunner() (Runner, ActiveLimits, error) { return nil, ActiveLimits{}, ErrNotLinux }
