//go:build linux

package plugins

import (
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Seccomp filter modes for a plugin start.
const (
	// netNone: no network sockets at all and every connect refused. For
	// plugins that list no hosts and are not handed the app's address.
	netNone = "none"
	// netNotify: TCP sockets allowed, every connect goes to the supervisor
	// in HolyArrs, which sends it to the proxy or the app. The network lock.
	netNotify = "notify"
	// netOpen: TCP sockets and connects allowed as they are. Only when the
	// system has no network lock and the owner chose to start plugins anyway;
	// UDP, raw sockets and the other refusals stay on.
	netOpen = "open"
)

const (
	retAllow  = unix.SECCOMP_RET_ALLOW
	retNotify = unix.SECCOMP_RET_USER_NOTIF
	retKill   = unix.SECCOMP_RET_KILL_PROCESS
)

func retErrno(e unix.Errno) uint32 { return unix.SECCOMP_RET_ERRNO | uint32(e) }

// seccomp_data offsets.
const (
	offNr   = 0
	offArch = 4
)

func offArgLo(i int) uint32 { return 16 + uint32(i)*8 }

// sysTable holds one architecture's syscall numbers for the filter. The
// numbers are written out per arch (not taken from this build's unix
// constants) so the filter for each arch can be built and checked on any
// host; TestSeccompTableMatchesBuild ties the native table to x/sys.
type sysTable struct {
	audit uint32
	// x32: amd64 kernels also accept x32 calls (bit 30 set) under the same
	// AUDIT_ARCH; they are killed.
	x32                                                    bool
	socket, socketpair, connect, sendto, sendmsg, sendmmsg uint32
	listen                                                 uint32
	// send is send(fd, buf, len, flags), only on 32-bit ARM; with
	// MSG_FASTOPEN it connects like sendto. 0 when the arch has none.
	send uint32
	// refused: multiplexed or legacy entry points that reach the socket
	// calls past their own numbers (socketcall on ARM OABI kernels).
	refused []uint32
	eperm   []uint32
}

var sysTables = map[string]sysTable{
	"amd64": {
		audit: 0xc000003e, x32: true,
		socket: 41, socketpair: 53, connect: 42, sendto: 44, sendmsg: 46, sendmmsg: 307, listen: 50,
		// io_uring x3, bpf, ptrace, process_vm_writev/readv, userfaultfd,
		// perf_event_open, pidfd_getfd, seccomp
		eperm: []uint32{425, 426, 427, 321, 101, 311, 310, 323, 298, 438, 317},
	},
	"arm64": {
		audit:  0xc00000b7,
		socket: 198, socketpair: 199, connect: 203, sendto: 206, sendmsg: 211, sendmmsg: 269, listen: 201,
		eperm: []uint32{425, 426, 427, 280, 117, 271, 270, 282, 241, 438, 277},
	},
	"arm": {
		audit:  0x40000028,
		socket: 281, socketpair: 288, connect: 283, sendto: 290, sendmsg: 296, sendmmsg: 374, listen: 284,
		send: 289,
		// 102: socketcall. EABI has no such call, but kernels built with
		// OABI_COMPAT run it with seccomp seeing nr 102 and AUDIT_ARCH_ARM.
		refused: []uint32{102},
		eperm:   []uint32{425, 426, 427, 386, 26, 377, 376, 388, 364, 438, 383},
	},
}

// auditArch is this build's AUDIT_ARCH value.
func auditArch() (uint32, error) {
	t, ok := sysTables[runtime.GOARCH]
	if !ok {
		return 0, fmt.Errorf("plugin network lock: no seccomp filter for %s", runtime.GOARCH)
	}
	return t.audit, nil
}

// bpfProg is a tiny assembler with forward labels for jumps.
type bpfProg struct {
	ins    []unix.SockFilter
	labels map[string]int
	fixes  []bpfFix
}

type bpfFix struct {
	at     int
	jt, jf string
}

func (p *bpfProg) stmt(code uint16, k uint32) {
	p.ins = append(p.ins, unix.SockFilter{Code: code, K: k})
}

// jump adds a conditional jump; "" for a side means the next instruction.
func (p *bpfProg) jump(code uint16, k uint32, jt, jf string) {
	p.fixes = append(p.fixes, bpfFix{at: len(p.ins), jt: jt, jf: jf})
	p.ins = append(p.ins, unix.SockFilter{Code: code, K: k})
}

func (p *bpfProg) label(name string) { p.labels[name] = len(p.ins) }

func (p *bpfProg) ret(k uint32) { p.stmt(unix.BPF_RET|unix.BPF_K, k) }

func (p *bpfProg) load(off uint32) { p.stmt(unix.BPF_LD|unix.BPF_W|unix.BPF_ABS, off) }

func (p *bpfProg) build() ([]unix.SockFilter, error) {
	for _, f := range p.fixes {
		for _, side := range []struct {
			name string
			dst  *uint8
		}{{f.jt, &p.ins[f.at].Jt}, {f.jf, &p.ins[f.at].Jf}} {
			if side.name == "" {
				continue
			}
			to, ok := p.labels[side.name]
			if !ok {
				return nil, fmt.Errorf("bpf: no label %q", side.name)
			}
			d := to - f.at - 1
			if d < 0 || d > 255 {
				return nil, fmt.Errorf("bpf: jump to %q out of range", side.name)
			}
			*side.dst = uint8(d)
		}
	}
	return p.ins, nil
}

// buildFilter returns the plugin's seccomp program for one mode.
func buildFilter(mode string) ([]unix.SockFilter, error) {
	return buildFilterFor(mode, runtime.GOARCH)
}

// buildFilterFor builds the program for one GOARCH's kernel ABI.
func buildFilterFor(mode, goarch string) ([]unix.SockFilter, error) {
	t, ok := sysTables[goarch]
	if !ok {
		return nil, fmt.Errorf("plugin network lock: no seccomp filter for %s", goarch)
	}
	jeq := uint16(unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K)
	jset := uint16(unix.BPF_JMP | unix.BPF_JSET | unix.BPF_K)
	p := &bpfProg{labels: map[string]int{}}
	p.load(offArch)
	p.jump(jeq, t.audit, "", "kill")
	p.load(offNr)
	if t.x32 {
		// x32 syscall numbers carry bit 30; they would skip every check below.
		p.jump(uint16(unix.BPF_JMP|unix.BPF_JGE|unix.BPF_K), 0x40000000, "kill", "")
	}
	p.jump(jeq, t.socket, "socket", "")
	p.jump(jeq, t.socketpair, "socketpair", "")
	p.jump(jeq, t.connect, "connect", "")
	p.jump(jeq, t.sendto, "flags3", "")
	p.jump(jeq, t.sendmsg, "flags2", "")
	p.jump(jeq, t.sendmmsg, "flags3", "")
	if t.send != 0 {
		p.jump(jeq, t.send, "flags3", "")
	}
	for _, nr := range t.refused {
		p.jump(jeq, nr, "eacces", "")
	}
	// A listening socket would let anyone who reaches the port talk to the
	// plugin past the host list; plugins never need one (tsnet listens in
	// its own userspace stack).
	p.jump(jeq, t.listen, "eacces", "")
	for _, nr := range t.eperm {
		p.jump(jeq, nr, "eperm", "")
	}
	p.ret(retAllow)

	// socket(family, type, protocol)
	p.label("socket")
	p.load(offArgLo(0))
	p.jump(jeq, unix.AF_UNIX, "unix", "")
	p.jump(jeq, unix.AF_NETLINK, "netlink", "")
	if mode != netNone {
		p.jump(jeq, unix.AF_INET, "inet", "")
		p.jump(jeq, unix.AF_INET6, "inet", "")
	}
	p.ret(retErrno(unix.EACCES))
	p.label("netlink")
	p.load(offArgLo(2))
	p.jump(jeq, unix.NETLINK_ROUTE, "allow", "eacces")
	// Unix datagram sockets can sendto a named socket without connect, so
	// only stream and seqpacket (whose connects the lock sees) are allowed.
	p.label("unix")
	p.load(offArgLo(1))
	p.stmt(unix.BPF_ALU|unix.BPF_AND|unix.BPF_K, 0xf) // SOCK_TYPE_MASK
	p.jump(jeq, unix.SOCK_STREAM, "allow", "")
	p.jump(jeq, unix.SOCK_SEQPACKET, "allow", "eacces")
	p.label("inet")
	p.load(offArgLo(1))
	p.stmt(unix.BPF_ALU|unix.BPF_AND|unix.BPF_K, 0xf) // SOCK_TYPE_MASK
	p.jump(jeq, unix.SOCK_STREAM, "", "eacces")
	p.load(offArgLo(2))
	p.jump(jeq, 0, "allow", "")
	p.jump(jeq, unix.IPPROTO_TCP, "allow", "eacces")

	p.label("socketpair")
	p.load(offArgLo(0))
	p.jump(jeq, unix.AF_UNIX, "allow", "eacces")

	p.label("connect")
	switch mode {
	case netNotify:
		p.ret(retNotify)
	case netOpen:
		p.ret(retAllow)
	default:
		p.ret(retErrno(unix.EACCES))
	}

	p.label("flags3")
	p.load(offArgLo(3))
	p.jump(jset, unix.MSG_FASTOPEN, "eacces", "allow")
	p.label("flags2")
	p.load(offArgLo(2))
	p.jump(jset, unix.MSG_FASTOPEN, "eacces", "allow")

	p.label("allow")
	p.ret(retAllow)
	p.label("eacces")
	p.ret(retErrno(unix.EACCES))
	p.label("eperm")
	p.ret(retErrno(unix.EPERM))
	p.label("kill")
	p.ret(retKill)
	return p.build()
}

// installFilter loads the filter on the calling thread (no_new_privs must
// be set). With notify it returns the listener fd, else -1.
func installFilter(mode string) (int, error) {
	ins, err := buildFilter(mode)
	if err != nil {
		return -1, err
	}
	prog := unix.SockFprog{Len: uint16(len(ins)), Filter: &ins[0]}
	flags := uintptr(0)
	if mode == netNotify {
		flags = unix.SECCOMP_FILTER_FLAG_NEW_LISTENER
	}
	fd, _, errno := unix.Syscall(unix.SYS_SECCOMP, unix.SECCOMP_SET_MODE_FILTER, flags, uintptr(unsafe.Pointer(&prog)))
	runtime.KeepAlive(ins)
	if errno != 0 {
		return -1, fmt.Errorf("seccomp filter: %w", errno)
	}
	if mode != netNotify {
		return -1, nil
	}
	return int(fd), nil
}
