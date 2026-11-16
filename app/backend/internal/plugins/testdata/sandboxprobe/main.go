// Command sandboxprobe tries one action and exits 0 only if it worked.
// Usage: sandboxprobe read|write <path> | signalparent | fork <pidfile> | sleep
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
	case "sleep":
		time.Sleep(time.Hour)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
