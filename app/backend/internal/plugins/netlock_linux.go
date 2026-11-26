//go:build linux

package plugins

import (
	"errors"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// netProbeArg starts this binary as the network lock probe child.
const netProbeArg = "__holyarrs-netlock-probe"

// Exit codes of the probe child.
const (
	probeOK        = 0
	probeOldKernel = 10
	probeBlocked   = 11
	probeTurnedOff = 12
	probeFailed    = 20
)

var (
	netProbeOnce   sync.Once
	netProbeOK     bool
	netProbeReason string
	// netProbeHelper is the binary the probe runs; empty means this one.
	netProbeHelper string
)

// netLockProbe checks once, in a throwaway child, that this system gives
// everything the network lock needs: a seccomp user-notify filter, reading
// the child's memory and putting a socket in its place (Linux 5.9). It returns false and
// FileLockOldKernel, FileLockBlocked or FileLockTurnedOff when not.
func netLockProbe() (bool, string) {
	netProbeOnce.Do(func() { netProbeOK, netProbeReason = runNetProbe() })
	return netProbeOK, netProbeReason
}

// NetLockReason is why the network lock is missing; empty when it works.
func NetLockReason() string {
	_, r := netLockProbe()
	return r
}

func errnoReason(err error) string {
	switch {
	case errors.Is(err, unix.EPERM), errors.Is(err, unix.EACCES):
		return FileLockBlocked
	}
	return FileLockOldKernel
}

func runNetProbe() (bool, string) {
	helper := netProbeHelper
	if helper == "" {
		self, err := os.Executable()
		if err != nil {
			return false, FileLockOldKernel
		}
		helper = self
	}
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return false, FileLockOldKernel
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return false, FileLockOldKernel
	}
	parent, child := os.NewFile(uintptr(fds[0]), "probe-parent"), os.NewFile(uintptr(fds[1]), "probe-child")
	defer parent.Close()
	cmd := exec.Command(helper, netProbeArg, strconv.Itoa(ln.Addr().(*net.TCPAddr).Port))
	cmd.Env = []string{}
	cmd.ExtraFiles = []*os.File{child}
	if err := cmd.Start(); err != nil {
		child.Close()
		return false, FileLockOldKernel
	}
	child.Close()
	exited := make(chan int, 1)
	go func() {
		err := cmd.Wait()
		code := 0
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		} else if err != nil {
			code = probeFailed
		}
		exited <- code
	}()
	fail := func(reason string) (bool, string) {
		_ = cmd.Process.Kill()
		<-exited
		return false, reason
	}
	lfd, err := recvFD(parent, 5*time.Second)
	if err != nil {
		select {
		case code := <-exited:
			return false, probeReason(code)
		case <-time.After(time.Second):
			return fail(FileLockOldKernel)
		}
	}
	defer unix.Close(lfd)
	pfds := []unix.PollFd{{Fd: int32(lfd), Events: unix.POLLIN}}
	if n, err := unix.Poll(pfds, 5000); err != nil || n == 0 {
		return fail(FileLockOldKernel)
	}
	var req seccompNotif
	if err := ioctlPtr(lfd, unix.SECCOMP_IOCTL_NOTIF_RECV, unsafe.Pointer(&req)); err != nil {
		return fail(errnoReason(err))
	}
	reply := func(errno unix.Errno) {
		resp := seccompResp{ID: req.ID, Error: -int32(errno)}
		_ = ioctlPtr(lfd, unix.SECCOMP_IOCTL_NOTIF_SEND, unsafe.Pointer(&resp))
	}
	dst, errno := readSockaddr(int(req.Pid), uintptr(req.Args[1]), int(uint32(req.Args[2])))
	if errno != 0 || !dst.IsValid() {
		reply(unix.EACCES)
		if errno == unix.EFAULT {
			return fail(FileLockBlocked)
		}
		return fail(FileLockOldKernel)
	}
	sockfd := int(int32(req.Args[0]))
	flags, err := fdFlags(int(req.Pid), sockfd)
	if err != nil {
		reply(unix.EACCES)
		return fail(FileLockBlocked)
	}
	to := netip.MustParseAddrPort(ln.Addr().String())
	// ADDFD needs Linux 5.9; an older kernel refuses it and the child's connect fails.
	errno = placeSocket(lfd, req.ID, sockfd, flags, false, to, dst.Port(), false, nil)
	reply(errno)
	select {
	case code := <-exited:
		if code == probeOK {
			return true, ""
		}
		return false, probeReason(code)
	case <-time.After(5 * time.Second):
		return fail(FileLockOldKernel)
	}
}

func probeReason(code int) string {
	switch code {
	case probeBlocked:
		return FileLockBlocked
	case probeTurnedOff:
		return FileLockTurnedOff
	}
	return FileLockOldKernel
}

// netProbeChild is the probe's child side: it installs a notify filter,
// sends the listener on fd 3 and connects once to the port in its argument.
func netProbeChild() int {
	runtime.LockOSThread()
	if _, _, e := unix.Syscall(unix.SYS_PRCTL, unix.PR_GET_SECCOMP, 0, 0); e == unix.EINVAL {
		return probeTurnedOff
	}
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return probeBlocked
	}
	lfd, err := installFilter(netNotify)
	if err != nil {
		if errnoReason(err) == FileLockBlocked {
			return probeBlocked
		}
		return probeOldKernel
	}
	if err := unix.Sendmsg(3, []byte{1}, unix.UnixRights(lfd), nil, 0); err != nil {
		return probeFailed
	}
	unix.Close(lfd)
	unix.Close(3)
	port, _ := strconv.Atoi(os.Args[len(os.Args)-1])
	s, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM, 0)
	if err != nil {
		return probeFailed
	}
	if err := unix.Connect(s, &unix.SockaddrInet4{Port: port, Addr: [4]byte{127, 0, 0, 1}}); err != nil {
		return probeFailed
	}
	return probeOK
}
