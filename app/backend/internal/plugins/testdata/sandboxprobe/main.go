// Command sandboxprobe tries one action and exits 0 only if it worked.
// Usage: sandboxprobe read|write <path> | signalparent | fork <pidfile> | escape <pidfile> | forkmany <n> | sleep
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

func main() {
	var err error
	switch os.Args[1] {
	case "read":
		_, err = os.ReadFile(os.Args[2])
	case "write":
		err = os.WriteFile(os.Args[2], []byte("x"), 0o600)
	case "signalparent":
		// SIGURG is harmless to a Go parent, so a missed lockdown does no damage.
		err = syscall.Kill(os.Getppid(), syscall.SIGURG)
	case "fork":
		child := exec.Command(os.Args[0], "sleep")
		// Inherit stdio: the sandbox may not open /dev/null for writing.
		child.Stdin, child.Stdout, child.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err = child.Start(); err == nil {
			err = os.WriteFile(os.Args[2], []byte(strconv.Itoa(child.Process.Pid)), 0o600)
		}
	case "escape":
		// A grandchild in its own session, out of the process group.
		child := exec.Command(os.Args[0], "sleep")
		child.Stdin, child.Stdout, child.Stderr = os.Stdin, os.Stdout, os.Stderr
		child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err = child.Start(); err == nil {
			err = os.WriteFile(os.Args[2], []byte(strconv.Itoa(child.Process.Pid)), 0o600)
		}
	case "forkmany":
		// Exits 0 only when a start fails before n children run.
		n, _ := strconv.Atoi(os.Args[2])
		err = fmt.Errorf("started all %d children", n)
		for i := 0; i < n; i++ {
			child := exec.Command(os.Args[0], "sleep")
			child.Stdin, child.Stdout, child.Stderr = os.Stdin, os.Stdout, os.Stderr
			if child.Start() != nil {
				err = nil
				break
			}
		}
	case "sleep":
		time.Sleep(time.Hour)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
