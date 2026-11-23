//go:build linux

package plugins

import (
	"runtime"
	"testing"

	"golang.org/x/sys/unix"
)

// runBPF runs a seccomp program on one call (classic BPF, the subset the
// filter uses).
func runBPF(t *testing.T, ins []unix.SockFilter, arch, nr uint32, args [6]uint64) uint32 {
	t.Helper()
	var a uint32
	word := func(off uint32) uint32 {
		switch {
		case off == offNr:
			return nr
		case off == offArch:
			return arch
		case off >= 16 && off < 64 && off%8 == 0:
			return uint32(args[(off-16)/8])
		case off >= 16 && off < 64:
			return uint32(args[(off-20)/8] >> 32)
		}
		t.Fatalf("load at %d", off)
		return 0
	}
	for pc := 0; pc < len(ins); pc++ {
		in := ins[pc]
		switch in.Code {
		case unix.BPF_LD | unix.BPF_W | unix.BPF_ABS:
			a = word(in.K)
		case unix.BPF_ALU | unix.BPF_AND | unix.BPF_K:
			a &= in.K
		case unix.BPF_RET | unix.BPF_K:
			return in.K
		case unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, unix.BPF_JMP | unix.BPF_JGE | unix.BPF_K, unix.BPF_JMP | unix.BPF_JSET | unix.BPF_K:
			var ok bool
			switch in.Code &^ (unix.BPF_JMP | unix.BPF_K) {
			case unix.BPF_JEQ:
				ok = a == in.K
			case unix.BPF_JGE:
				ok = a >= in.K
			default:
				ok = a&in.K != 0
			}
			if ok {
				pc += int(in.Jt)
			} else {
				pc += int(in.Jf)
			}
		default:
			t.Fatalf("op %#x at %d", in.Code, pc)
		}
	}
	t.Fatal("ran off the end")
	return 0
}

func TestSeccompTableMatchesBuild(t *testing.T) {
	tb, ok := sysTables[runtime.GOARCH]
	if !ok {
		t.Skip("no table for", runtime.GOARCH)
	}
	want := sysTable{socket: unix.SYS_SOCKET, socketpair: unix.SYS_SOCKETPAIR, connect: unix.SYS_CONNECT,
		sendto: unix.SYS_SENDTO, sendmsg: unix.SYS_SENDMSG, sendmmsg: unix.SYS_SENDMMSG, listen: unix.SYS_LISTEN}
	got := tb
	got.audit, got.x32, got.send, got.refused, got.eperm = 0, false, 0, nil, nil
	if got.socket != want.socket || got.socketpair != want.socketpair || got.connect != want.connect ||
		got.sendto != want.sendto || got.sendmsg != want.sendmsg || got.sendmmsg != want.sendmmsg || got.listen != want.listen {
		t.Fatalf("table %+v, x/sys %+v", got, want)
	}
	eperm := []uint32{unix.SYS_IO_URING_SETUP, unix.SYS_IO_URING_ENTER, unix.SYS_IO_URING_REGISTER, unix.SYS_BPF,
		unix.SYS_PTRACE, unix.SYS_PROCESS_VM_WRITEV, unix.SYS_PROCESS_VM_READV, unix.SYS_USERFAULTFD,
		unix.SYS_PERF_EVENT_OPEN, unix.SYS_PIDFD_GETFD, unix.SYS_SECCOMP}
	for i, nr := range eperm {
		if tb.eperm[i] != nr {
			t.Errorf("eperm[%d] = %d, x/sys %d", i, tb.eperm[i], nr)
		}
	}
	arch, _ := auditArch()
	if arch != tb.audit {
		t.Errorf("audit %#x", arch)
	}
}

func TestSeccompFilterPerArch(t *testing.T) {
	eacces := retErrno(unix.EACCES)
	audits := map[string]uint32{"amd64": unix.AUDIT_ARCH_X86_64, "arm64": unix.AUDIT_ARCH_AARCH64, "arm": unix.AUDIT_ARCH_ARM}
	foreign := []uint32{unix.AUDIT_ARCH_I386, unix.AUDIT_ARCH_X86_64, unix.AUDIT_ARCH_AARCH64, unix.AUDIT_ARCH_ARM, unix.AUDIT_ARCH_ARMEB, 0}
	for goarch, tb := range sysTables {
		if audits[goarch] != tb.audit {
			t.Fatalf("%s audit %#x", goarch, tb.audit)
		}
		for _, mode := range []string{netNone, netNotify, netOpen} {
			ins, err := buildFilterFor(mode, goarch)
			if err != nil {
				t.Fatal(err)
			}
			run := func(nr uint32, args ...uint64) uint32 {
				var a [6]uint64
				copy(a[:], args)
				return runBPF(t, ins, tb.audit, nr, a)
			}
			name := goarch + "/" + mode
			for _, fa := range foreign {
				if fa != tb.audit && runBPF(t, ins, fa, tb.socket, [6]uint64{unix.AF_INET, unix.SOCK_STREAM}) != retKill {
					t.Errorf("%s: arch %#x not killed", name, fa)
				}
			}
			if tb.x32 {
				for _, nr := range []uint32{0x40000000 | tb.socket, 0x40000000 | tb.connect, 0x40000000 + 545} {
					if run(nr) != retKill {
						t.Errorf("%s: x32 nr %#x not killed", name, nr)
					}
				}
			}
			if goarch == "arm" {
				// socketcall(SYS_CONNECT=3, args)
				if got := run(102, 3, 0); got != eacces {
					t.Errorf("%s: socketcall -> %#x", name, got)
				}
				if got := run(tb.send, 3, 0, 1, unix.MSG_FASTOPEN); got != eacces {
					t.Errorf("%s: send fastopen -> %#x", name, got)
				}
				if got := run(tb.send, 3, 0, 1, 0); got != retAllow {
					t.Errorf("%s: plain send -> %#x", name, got)
				}
			}
			if got := run(tb.sendto, 3, 0, 1, unix.MSG_FASTOPEN); got != eacces {
				t.Errorf("%s: sendto fastopen -> %#x", name, got)
			}
			if got := run(tb.listen, 3, 1); got != eacces {
				t.Errorf("%s: listen -> %#x", name, got)
			}
			if got := run(tb.socket, unix.AF_INET, unix.SOCK_DGRAM); got != eacces {
				t.Errorf("%s: udp -> %#x", name, got)
			}
			for _, nr := range tb.eperm {
				if got := run(nr); got != retErrno(unix.EPERM) {
					t.Errorf("%s: nr %d -> %#x", name, nr, got)
				}
			}
			wantConn := map[string]uint32{netNone: eacces, netNotify: retNotify, netOpen: retAllow}[mode]
			if got := run(tb.connect, 3); got != wantConn {
				t.Errorf("%s: connect -> %#x", name, got)
			}
			wantTCP := uint32(retAllow)
			if mode == netNone {
				wantTCP = eacces
			}
			if got := run(tb.socket, unix.AF_INET, unix.SOCK_STREAM, 0); got != wantTCP {
				t.Errorf("%s: tcp socket -> %#x", name, got)
			}
		}
	}
}
