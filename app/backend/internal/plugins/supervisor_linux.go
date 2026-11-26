//go:build linux

package plugins

import (
	"encoding/binary"
	"errors"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/unix"
)

// seccompNotif is struct seccomp_notif.
type seccompNotif struct {
	ID    uint64
	Pid   uint32
	Flags uint32
	Nr    int32
	Arch  uint32
	IP    uint64
	Args  [6]uint64
}

// seccompResp is struct seccomp_notif_resp.
type seccompResp struct {
	ID    uint64
	Val   int64
	Error int32
	Flags uint32
}

// seccompAddfd is struct seccomp_notif_addfd.
type seccompAddfd struct {
	ID         uint64
	Flags      uint32
	SrcFD      uint32
	NewFD      uint32
	NewFDFlags uint32
}

// netSupervisor answers one plugin's connects. It never lets the plugin's
// own connect through: it opens a socket of its own, connects it to the app
// (self) or the plugin's proxy, and puts it in the plugin's place at the
// same fd number (SECCOMP_IOCTL_NOTIF_ADDFD with SETFD), then reports
// success. Go registers a socket with its poller only after connect returns,
// so a Go plugin uses the new socket as if it had connected it itself. The
// address the plugin passed is read only as a hint, so there is no
// check-then-use race.
type netSupervisor struct {
	fd    int
	self  netip.AddrPort // the app's address the plugin may reach directly; zero for none
	proxy *ProxyHandle   // nil: every other inet connect is refused
	stop  atomic.Bool
	done  chan struct{}
}

func startSupervisor(fd int, self netip.AddrPort, proxy *ProxyHandle) *netSupervisor {
	s := &netSupervisor{fd: fd, self: self, proxy: proxy, done: make(chan struct{})}
	go s.loop()
	return s
}

// Close ends the loop and closes the listener; the kernel then fails every
// connect the plugin still tries.
func (s *netSupervisor) Close() {
	if s.stop.Swap(true) {
		return
	}
	<-s.done
}

func (s *netSupervisor) loop() {
	defer close(s.done)
	defer unix.Close(s.fd)
	for !s.stop.Load() {
		fds := []unix.PollFd{{Fd: int32(s.fd), Events: unix.POLLIN}}
		n, err := unix.Poll(fds, 500)
		if err != nil {
			if errors.Is(err, unix.EINTR) {
				continue
			}
			return
		}
		if n == 0 {
			continue
		}
		if fds[0].Revents&(unix.POLLHUP|unix.POLLERR|unix.POLLNVAL) != 0 && fds[0].Revents&unix.POLLIN == 0 {
			return // no task uses the filter any more
		}
		var req seccompNotif
		if err := ioctlPtr(s.fd, unix.SECCOMP_IOCTL_NOTIF_RECV, unsafe.Pointer(&req)); err != nil {
			if errors.Is(err, unix.EINTR) || errors.Is(err, unix.ENOENT) {
				continue
			}
			return
		}
		errno := s.decide(&req)
		resp := seccompResp{ID: req.ID, Error: -int32(errno)}
		_ = ioctlPtr(s.fd, unix.SECCOMP_IOCTL_NOTIF_SEND, unsafe.Pointer(&resp))
	}
}

func ioctlPtr(fd int, req uint, p unsafe.Pointer) error {
	_, _, e := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(req), uintptr(p))
	if e != 0 {
		return e
	}
	return nil
}

func (s *netSupervisor) valid(id uint64) bool {
	return ioctlPtr(s.fd, unix.SECCOMP_IOCTL_NOTIF_ID_VALID, unsafe.Pointer(&id)) == nil
}

// decide handles one connect and returns the errno for the plugin, 0 on success.
func (s *netSupervisor) decide(req *seccompNotif) unix.Errno {
	if req.Nr != unix.SYS_CONNECT {
		return unix.EACCES
	}
	sockfd, ptr, length := int(int32(req.Args[0])), uintptr(req.Args[1]), int(uint32(req.Args[2]))
	dst, errno := readSockaddr(int(req.Pid), ptr, length)
	if errno != 0 {
		return errno
	}
	var to netip.AddrPort
	viaProxy := false
	switch {
	case !dst.IsValid():
		return unix.EACCES // not an inet address: unix, abstract unix, anything else
	case s.self.IsValid() && dst.Addr().Unmap() == s.self.Addr() && dst.Port() == s.self.Port():
		to = s.self
	case s.proxy != nil:
		to, viaProxy = s.proxy.Addr(), true
	default:
		return unix.EACCES
	}
	flags, err := fdFlags(int(req.Pid), sockfd)
	if err != nil {
		return unix.EBADF
	}
	if !s.valid(req.ID) {
		return unix.EACCES
	}
	return placeSocket(s.fd, req.ID, sockfd, flags, dst.Addr().Is6() && !dst.Addr().Is4In6(), to, dst.Port(), viaProxy, s.proxy)
}

// placeSocket connects a new socket of ours to `to` and puts it at the
// plugin's fd number, with the plugin's O_NONBLOCK and O_CLOEXEC. For the
// proxy it first notes which port the plugin asked for under the socket's
// local port.
func placeSocket(notifFD int, id uint64, target int, flags int, v6 bool, to netip.AddrPort, want uint16, viaProxy bool, proxy *ProxyHandle) unix.Errno {
	family := unix.AF_INET
	var sa unix.Sockaddr = &unix.SockaddrInet4{Port: int(to.Port()), Addr: to.Addr().As4()}
	if v6 {
		family = unix.AF_INET6
		sa = &unix.SockaddrInet6{Port: int(to.Port()), Addr: to.Addr().As16()}
	}
	sock, err := unix.Socket(family, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return unix.ENOBUFS
	}
	defer unix.Close(sock)
	if err := unix.Connect(sock, sa); err != nil {
		var errno unix.Errno
		if errors.As(err, &errno) {
			return errno
		}
		return unix.ECONNREFUSED
	}
	if viaProxy {
		if local, err := unix.Getsockname(sock); err == nil {
			switch a := local.(type) {
			case *unix.SockaddrInet4:
				proxy.Expect(a.Port, int(want))
			case *unix.SockaddrInet6:
				proxy.Expect(a.Port, int(want))
			}
		}
	}
	if flags&unix.O_NONBLOCK != 0 {
		if err := unix.SetNonblock(sock, true); err != nil {
			return unix.EACCES
		}
	}
	add := seccompAddfd{ID: id, Flags: unix.SECCOMP_ADDFD_FLAG_SETFD, SrcFD: uint32(sock), NewFD: uint32(target)}
	if flags&unix.O_CLOEXEC != 0 {
		add.NewFDFlags = unix.O_CLOEXEC
	}
	if err := ioctlPtr(notifFD, seccompIoctlAddfd, unsafe.Pointer(&add)); err != nil {
		return unix.EACCES
	}
	return 0
}

// seccompIoctlAddfd is SECCOMP_IOCTL_NOTIF_ADDFD.
const seccompIoctlAddfd = 0x40182103

// fdFlags reads the open flags of a plugin fd from /proc.
func fdFlags(tid, fd int) (int, error) {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(tid) + "/fdinfo/" + strconv.Itoa(fd))
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(b), "\n") {
		if v, ok := strings.CutPrefix(line, "flags:"); ok {
			n, err := strconv.ParseInt(strings.TrimSpace(v), 8, 64)
			return int(n), err
		}
	}
	return 0, errors.New("no flags")
}

// readSockaddr reads the plugin's sockaddr; it returns an invalid AddrPort
// for any family other than inet.
func readSockaddr(pid int, ptr uintptr, length int) (netip.AddrPort, unix.Errno) {
	if length < 2 || ptr == 0 {
		return netip.AddrPort{}, unix.EINVAL
	}
	if length > 128 {
		length = 128
	}
	buf := make([]byte, length)
	local := []unix.Iovec{{Base: &buf[0]}}
	local[0].SetLen(length)
	remote := []unix.RemoteIovec{{Base: ptr, Len: length}}
	n, err := unix.ProcessVMReadv(pid, local, remote, 0)
	if err != nil || n < 2 {
		return netip.AddrPort{}, unix.EFAULT
	}
	buf = buf[:n]
	family := binary.NativeEndian.Uint16(buf[:2])
	switch family {
	case unix.AF_INET:
		if len(buf) < 8 {
			return netip.AddrPort{}, unix.EINVAL
		}
		return netip.AddrPortFrom(netip.AddrFrom4([4]byte(buf[4:8])), binary.BigEndian.Uint16(buf[2:4])), 0
	case unix.AF_INET6:
		if len(buf) < 24 {
			return netip.AddrPort{}, unix.EINVAL
		}
		return netip.AddrPortFrom(netip.AddrFrom16([16]byte(buf[8:24])), binary.BigEndian.Uint16(buf[2:4])), 0
	}
	return netip.AddrPort{}, 0
}
