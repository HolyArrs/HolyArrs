//go:build linux

// Linux lockdown for plugin programs.
//
// Design: Landlock and no_new_privs apply to the calling thread and are kept
// across exec, but os/exec gives no hook that runs in the child between fork
// and exec. So LinuxSandbox.Apply rewrites the command to run our own binary
// in a hidden helper mode (first argument sandboxHelperArg). The helper, still
// running as the same non-root user, locks its thread, sets no_new_privs,
// sets rlimits, restricts itself with Landlock and then execs the plugin run
// copy in place, so the plugin starts already locked down. The binary that
// hosts plugins must call RunSandboxHelper first thing in main (tests call it
// from TestMain). Nothing here needs root.
//
// Filesystem (Landlock): the plugin may read and write its data folder, read
// and execute its run copy, and read /etc/ssl/certs, /etc/resolv.conf,
// /etc/hosts, /dev/null and /dev/urandom. Every other path is denied. When
// the kernel has no Landlock, starting fails unless AllowWeaker is set.
//
// Also: no_new_privs; its own process group, killed as a whole on stop;
// rlimits open files 1024 and no core dumps (address space is left alone,
// Go needs a large one).
//
// With Landlock ABI 6 or newer the plugin also cannot signal processes or
// reach abstract unix sockets outside its own sandbox. GO* variables
// (GODEBUG, GOTRACEBACK, GOMAXPROCS...) are dropped from its environment.
//
// Resources (cgroup v2): when the app's own cgroup is delegated to it, each
// start gets a child cgroup holyarrs-plugin-<id>-<random> with pids.max 256,
// memory.max 512 MiB (Spec.Limits overrides) and cpu.max of 1 CPU. The helper
// joins it before the lockdown, so the plugin and everything it starts stay
// inside; Stop writes cgroup.kill, which no setsid or double fork escapes,
// then removes the folder. Without a usable cgroup the start goes on with
// the process group kill only; Active reports which caps apply.
//
// Known limits: no network allowlist (Landlock TCP port rules may come
// later). A cgroup whose program ended on its own is killed and removed then.
package plugins

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

const sandboxHelperArg = "__holyarrs-plugin-sandbox"

// landlockRulePathBeneath is LANDLOCK_RULE_PATH_BENEATH (x/sys has no name for it).
const landlockRulePathBeneath = 1

// sandboxReadOnly is the fixed list of system paths a plugin may read.
var sandboxReadOnly = []string{"/etc/ssl/certs", "/etc/resolv.conf", "/etc/hosts", "/dev/null", "/dev/urandom"}

// ErrNoLandlock means the kernel cannot apply the filesystem lockdown.
var ErrNoLandlock = errors.New("plugin sandbox: kernel has no Landlock")

// LinuxSandbox locks plugin programs down with Landlock, no_new_privs,
// rlimits and a process group of their own.
type LinuxSandbox struct {
	// Helper is the binary that runs RunSandboxHelper; empty means this one.
	Helper string
	// AllowWeaker starts plugins without Landlock when the kernel lacks it.
	AllowWeaker bool
}

// Apply turns cmd into a helper start that locks itself down and then execs
// the plugin.
func (s LinuxSandbox) Apply(cmd *exec.Cmd, spec Spec) error {
	if landlockABI() < 1 && !s.AllowWeaker {
		return ErrNoLandlock
	}
	helper := s.Helper
	if helper == "" {
		self, err := os.Executable()
		if err != nil {
			return fmt.Errorf("plugin sandbox helper: %w", err)
		}
		helper = self
	}
	weaker := "0"
	if s.AllowWeaker {
		weaker = "1"
	}
	env := cmd.Env[:0:0]
	for _, kv := range cmd.Env {
		if !strings.HasPrefix(kv, "GO") {
			env = append(env, kv)
		}
	}
	cmd.Env = env
	cmd.Path = helper
	cg := ""
	if cgroupUsable() {
		var err error
		if cg, err = cgroupCreate(spec.ID, spec.Limits); err != nil {
			return err
		}
	}
	cmd.Args = append([]string{helper, sandboxHelperArg, spec.DataDir, weaker, cg, spec.Program}, spec.Args...)
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	var once sync.Once
	var cancelErr error
	cmd.Cancel = func() error {
		once.Do(func() {
			cancelErr = killProcess(cmd)
			if cg != "" {
				cancelErr = cgroupKill(cg)
			}
		})
		return cancelErr
	}
	return nil
}

// Cleanup removes a start's cgroup when Start failed or the program ended
// on its own, killing anything it left behind.
func (LinuxSandbox) Cleanup(cmd *exec.Cmd) error {
	if len(cmd.Args) > 4 && cmd.Args[1] == sandboxHelperArg && cmd.Args[4] != "" {
		return cgroupKill(cmd.Args[4])
	}
	return nil
}

// Active reports which resource caps and stop guarantees this host gives.
func (LinuxSandbox) Active() ActiveLimits { return cgroupActive() }

// RunSandboxHelper does nothing unless this process was started as the
// sandbox helper; then it locks down and execs the plugin, never returning.
func RunSandboxHelper() {
	if len(os.Args) < 6 || os.Args[1] != sandboxHelperArg {
		return
	}
	err := sandboxExec(os.Args[2], os.Args[3] == "1", os.Args[4], os.Args[5], os.Args[6:])
	fmt.Fprintln(os.Stderr, "plugin sandbox:", err)
	os.Exit(126)
}

func sandboxExec(dataDir string, allowWeaker bool, cgroup, program string, args []string) error {
	if cgroup != "" {
		if err := cgroupJoin(cgroup); err != nil {
			return fmt.Errorf("join cgroup: %w", err)
		}
	}
	runtime.LockOSThread() // Landlock and no_new_privs are per thread; exec from this one.
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("no_new_privs: %w", err)
	}
	// syscall.Setrlimit (not unix) so Go does not put back its raised file limit on exec.
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &syscall.Rlimit{Cur: 1024, Max: 1024}); err != nil {
		return fmt.Errorf("rlimit open files: %w", err)
	}
	if err := syscall.Setrlimit(syscall.RLIMIT_CORE, &syscall.Rlimit{}); err != nil {
		return fmt.Errorf("rlimit core: %w", err)
	}
	if err := landlockRestrict(dataDir, program); err != nil {
		if !errors.Is(err, ErrNoLandlock) || !allowWeaker {
			return err
		}
	}
	return unix.Exec(program, append([]string{program}, args...), os.Environ())
}

// landlockABI returns the kernel's Landlock ABI version, 0 when it has none.
func landlockABI() int {
	v, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0, unix.LANDLOCK_CREATE_RULESET_VERSION)
	if errno != 0 {
		return 0
	}
	return int(v)
}

const (
	llFileRead = unix.LANDLOCK_ACCESS_FS_READ_FILE
	llDirRead  = unix.LANDLOCK_ACCESS_FS_READ_FILE | unix.LANDLOCK_ACCESS_FS_READ_DIR
	llFileRW   = unix.LANDLOCK_ACCESS_FS_READ_FILE | unix.LANDLOCK_ACCESS_FS_WRITE_FILE | unix.LANDLOCK_ACCESS_FS_TRUNCATE
	llDirRW    = llFileRW | unix.LANDLOCK_ACCESS_FS_READ_DIR | unix.LANDLOCK_ACCESS_FS_REMOVE_DIR |
		unix.LANDLOCK_ACCESS_FS_REMOVE_FILE | unix.LANDLOCK_ACCESS_FS_MAKE_DIR | unix.LANDLOCK_ACCESS_FS_MAKE_REG |
		unix.LANDLOCK_ACCESS_FS_MAKE_SYM | unix.LANDLOCK_ACCESS_FS_REFER
)

// landlockHandled is every filesystem right the ABI knows, so each is
// denied unless a rule allows it.
func landlockHandled(abi int) uint64 {
	h := uint64(unix.LANDLOCK_ACCESS_FS_EXECUTE | unix.LANDLOCK_ACCESS_FS_WRITE_FILE | unix.LANDLOCK_ACCESS_FS_READ_FILE |
		unix.LANDLOCK_ACCESS_FS_READ_DIR | unix.LANDLOCK_ACCESS_FS_REMOVE_DIR | unix.LANDLOCK_ACCESS_FS_REMOVE_FILE |
		unix.LANDLOCK_ACCESS_FS_MAKE_CHAR | unix.LANDLOCK_ACCESS_FS_MAKE_DIR | unix.LANDLOCK_ACCESS_FS_MAKE_REG |
		unix.LANDLOCK_ACCESS_FS_MAKE_SOCK | unix.LANDLOCK_ACCESS_FS_MAKE_FIFO | unix.LANDLOCK_ACCESS_FS_MAKE_BLOCK |
		unix.LANDLOCK_ACCESS_FS_MAKE_SYM)
	if abi >= 2 {
		h |= unix.LANDLOCK_ACCESS_FS_REFER
	}
	if abi >= 3 {
		h |= unix.LANDLOCK_ACCESS_FS_TRUNCATE
	}
	if abi >= 5 {
		h |= unix.LANDLOCK_ACCESS_FS_IOCTL_DEV
	}
	return h
}

func landlockRestrict(dataDir, program string) error {
	// Never grant anything under the cgroup tree, so the plugin cannot raise its own caps.
	for _, p := range []string{dataDir, program} {
		if c := filepath.Clean(p); c == cgroupRoot || strings.HasPrefix(c, cgroupRoot+"/") {
			return fmt.Errorf("landlock %s: paths under %s are not allowed", p, cgroupRoot)
		}
	}
	abi := landlockABI()
	if abi < 1 {
		return ErrNoLandlock
	}
	handled := landlockHandled(abi)
	attr := unix.LandlockRulesetAttr{Handled_access_fs: handled}
	if abi >= 6 {
		attr.Scoped = unix.LANDLOCK_SCOPE_SIGNAL | unix.LANDLOCK_SCOPE_ABSTRACT_UNIX_SOCKET
	}
	fd, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, uintptr(unsafe.Pointer(&attr)), unsafe.Sizeof(attr), 0)
	if errno != 0 {
		return fmt.Errorf("landlock ruleset: %w", errno)
	}
	defer unix.Close(int(fd))
	add := func(path string, dirAccess, fileAccess uint64, required bool) error {
		f, err := os.OpenFile(path, unix.O_PATH|unix.O_CLOEXEC, 0)
		if err != nil {
			if !required && errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return fmt.Errorf("landlock %s: %w", path, err)
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil {
			return fmt.Errorf("landlock %s: %w", path, err)
		}
		access := fileAccess
		if st.IsDir() {
			access = dirAccess
		}
		rule := unix.LandlockPathBeneathAttr{Allowed_access: access & handled, Parent_fd: int32(f.Fd())}
		_, _, errno := unix.Syscall6(unix.SYS_LANDLOCK_ADD_RULE, fd, landlockRulePathBeneath, uintptr(unsafe.Pointer(&rule)), 0, 0, 0)
		if errno != 0 {
			return fmt.Errorf("landlock %s: %w", path, errno)
		}
		return nil
	}
	if err := add(dataDir, llDirRW, llFileRW, true); err != nil {
		return err
	}
	if err := add(program, 0, llFileRead|unix.LANDLOCK_ACCESS_FS_EXECUTE, true); err != nil {
		return err
	}
	for _, p := range sandboxReadOnly {
		if err := add(p, llDirRead, llFileRead, false); err != nil {
			return err
		}
	}
	if _, _, errno := unix.Syscall(unix.SYS_LANDLOCK_RESTRICT_SELF, fd, 0, 0); errno != 0 {
		return fmt.Errorf("landlock restrict: %w", errno)
	}
	return nil
}

// killProcess kills the whole process group when the program has its own,
// even after the main process has ended, so children it left are killed too.
func killProcess(cmd *exec.Cmd) error {
	if cmd.SysProcAttr != nil && cmd.SysProcAttr.Setpgid && cmd.Process != nil {
		return unix.Kill(-cmd.Process.Pid, unix.SIGKILL)
	}
	return cmd.Process.Kill()
}

// DefaultRunner runs plugins in the LinuxSandbox and reports its caps.
func DefaultRunner() (Runner, ActiveLimits, error) {
	sb := LinuxSandbox{}
	return ExecRunner{Sandbox: sb}, sb.Active(), nil
}
