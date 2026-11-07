package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

const (
	envTestExec        = "HOLYARRS_TEST_EXEC"
	envTestReexecChild = "HOLYARRS_TEST_REEXEC_CHILD"
)

// testCommand starts a Go test or binary through the userspace emulator named
// by HOLYARRS_TEST_EXEC. Native tests leave the variable empty.
func testCommand(target string, args ...string) *exec.Cmd {
	if testExec := os.Getenv(envTestExec); testExec != "" {
		return exec.Command(testExec, append([]string{target}, args...)...)
	}
	return exec.Command(target, args...)
}

func TestTestCommandReexec(t *testing.T) {
	if os.Getenv(envTestReexecChild) != "" {
		fmt.Println("test-command-child-ok")
		return
	}

	cmd := testCommand(os.Args[0], "-test.run=^TestTestCommandReexec$", "-test.count=1")
	cmd.Env = append(os.Environ(), envTestReexecChild+"=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("reexec test child: %v\n%s", err, out)
	}
	if want := "test-command-child-ok"; !strings.Contains(string(out), want+"\n") {
		t.Fatalf("reexec test child did not report %q:\n%s", want, out)
	}
}
