package youtube

import "os/exec"

func startOwnGroup(*exec.Cmd) {}

func killGroup(cmd *exec.Cmd) error {
	return cmd.Process.Kill()
}
