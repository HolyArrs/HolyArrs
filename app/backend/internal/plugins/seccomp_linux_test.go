//go:build linux

package plugins

import (
	"net"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// seccompChild runs one escape attempt under the real filter and exits 0
// only when the filter refused it.
func seccompChild() {
	runtime.LockOSThread()
	unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0)
	if _, err := installFilter(netNotify); err != nil {
		os.Exit(50)
	}
	switch os.Getenv("SECCOMP_TRY") {
	case "dgram":
		s, err := unix.Socket(unix.AF_UNIX, unix.SOCK_DGRAM, 0)
		if err == nil {
			unix.Sendto(s, []byte("exfil"), 0, &unix.SockaddrUnix{Name: "@holyarrs-seccomp-dgram"})
			os.Exit(1)
		}
		if s, err := unix.Socket(unix.AF_UNIX, unix.SOCK_STREAM, 0); err != nil {
			os.Exit(2)
		} else {
			unix.Close(s)
		}
	case "socketcall":
		// socketcall(SYS_SOCKET=1, [AF_INET, SOCK_STREAM, 0]) on 32-bit ARM.
		args := [3]uintptr{unix.AF_INET, unix.SOCK_STREAM, 0}
		_, _, e := unix.Syscall(102, 1, uintptr(unsafe.Pointer(&args)), 0)
		if e != unix.EACCES {
			os.Exit(int(e) + 100)
		}
	case "listen":
		s, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM, 0)
		if err != nil {
			os.Exit(3)
		}
		unix.Bind(s, &unix.SockaddrInet4{})
		if unix.Listen(s, 1) == nil {
			os.Exit(1)
		}
	}
	os.Exit(0)
}

func init() {
	if os.Getenv("SECCOMP_CHILD") == "1" {
		seccompChild()
	}
}

func runSeccompChild(t *testing.T, try string) error {
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = []string{"SECCOMP_CHILD=1", "SECCOMP_TRY=" + try}
	return cmd.Run()
}

func TestSeccompRefusesUnixDatagram(t *testing.T) {
	ln, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: "@holyarrs-seccomp-dgram", Net: "unixgram"})
	if err != nil {
		t.Skip(err)
	}
	defer ln.Close()
	if err := runSeccompChild(t, "dgram"); err != nil {
		t.Fatalf("child: %v", err)
	}
	ln.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if _, _, err := ln.ReadFrom(make([]byte, 16)); err == nil {
		t.Error("a datagram reached a unix socket outside the sandbox")
	}
}

func TestSeccompRefusesListen(t *testing.T) {
	if err := runSeccompChild(t, "listen"); err != nil {
		t.Fatalf("child: %v", err)
	}
}

func TestSeccompRefusesSocketcall(t *testing.T) {
	if runtime.GOARCH != "arm" {
		t.Skip("socketcall check is for 32-bit ARM")
	}
	if err := runSeccompChild(t, "socketcall"); err != nil {
		t.Fatalf("child: %v", err)
	}
}
