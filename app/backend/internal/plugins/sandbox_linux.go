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
// Landlock is missing, starting fails closed with ErrNoLandlock. The owner
// can allow one signed-list plugin to start without it on the Plugins page
// (Spec.AllowNoFileLock); the other limits stay on and the log says so.
// Why it is missing decides what the page suggests: an old kernel (ENOSYS),
// a container profile that blocks it (EPERM) or a boot setting that turns
// it off (EOPNOTSUPP).
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
// Network (seccomp, see seccomp_linux.go and supervisor_linux.go): a plugin
// that lists no hosts and is not handed the app's address gets no network
// sockets. Otherwise every connect is caught by a seccomp user-notify filter
// and answered by a supervisor in HolyArrs, which connects the plugin's own
// socket to the app's address or to the plugin's proxy (netproxy.go); the
// proxy lets through only the listed names. UDP, raw sockets, io_uring and
// TCP fast open are refused for every plugin. Without user-notify (kernel
// below 5.9, or blocked) a plugin with hosts starts only when the owner
// allowed it (Spec.AllowNoNetLock); the other refusals stay on.
//
// Folders: each folder the owner approved is opened by HolyArrs and passed
// to the helper as an open file, which gets the Landlock rule, so its path is
// never looked up again. Read-write folders cannot get symlinks, fifos,
// sockets or device files.
//
// Known limits: a cgroup whose program ended on its own is killed and
// removed then.
package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

const sandboxHelperArg = "__holyarrs-plugin-sandbox"

// landlockRulePathBeneath is LANDLOCK_RULE_PATH_BENEATH (x/sys has no name for it).
const landlockRulePathBeneath = 1

// sandboxReadOnly is the fixed list of system paths a plugin may read.
var sandboxReadOnly = []string{"/etc/ssl/certs", "/etc/resolv.conf", "/etc/hosts", "/dev/null", "/dev/urandom"}

// LinuxSandbox locks plugin programs down with Landlock, seccomp,
// no_new_privs, rlimits and a process group of their own.
type LinuxSandbox struct {
	// Helper is the binary that runs RunSandboxHelper; empty means this one.
	Helper string
	// AllowWeaker starts every plugin without Landlock when it is missing.
	// Only tests set it; real starts use the per-plugin Spec.AllowNoFileLock.
	AllowWeaker bool
	// NetLock is true when the system gives seccomp user-notify (the
	// network lock); DefaultRunner sets it from netLockProbe.
	NetLock bool
}

// helperConfig is what the helper gets as its one argument.
type helperConfig struct {
	DataDir string `json:"data"`
	Weaker  bool   `json:"weaker,omitempty"`
	Cgroup  string `json:"cgroup,omitempty"`
	// OpenFiles is the open-file limit.
	OpenFiles uint64         `json:"open_files,omitempty"`
	Net       string         `json:"net"`
	Folders   []helperFolder `json:"folders,omitempty"`
	// NetBestEffort lets the start go on when the filter cannot load (the
	// owner chose to run without the network lock).
	NetBestEffort bool `json:"net_best_effort,omitempty"`
}

// helperFolder is one approved folder, already open at fd FD in the helper.
type helperFolder struct {
	FD    int  `json:"fd"`
	Write bool `json:"write,omitempty"`
}

// sbState is what Started and Cleanup need about one start.
type sbState struct {
	cfg    helperConfig
	parent *os.File // our end of the socketpair the helper sends the listener on
	child  *os.File
	self   netip.AddrPort
	proxy  *ProxyHandle
	sup    *netSupervisor
}

var sbStates sync.Map // *exec.Cmd -> *sbState

// netMode picks the filter for one start.
func (s LinuxSandbox) netMode(spec Spec) (mode string, bestEffort bool, err error) {
	if spec.Proxy == nil && spec.SelfAddr == "" {
		return netNone, false, nil
	}
	if s.NetLock {
		return netNotify, false, nil
	}
	if spec.AllowNoNetLock {
		return netOpen, true, nil
	}
	return "", false, ErrNoNetLock
}

// Apply turns cmd into a helper start that locks itself down and then execs
// the plugin.
func (s LinuxSandbox) Apply(cmd *exec.Cmd, spec Spec) error {
	weakerOK := s.AllowWeaker || spec.AllowNoFileLock
	if landlockABI() < 1 && !weakerOK {
		return ErrNoLandlock
	}
	mode, best, err := s.netMode(spec)
	if err != nil {
		return err
	}
	st := &sbState{cfg: helperConfig{DataDir: spec.DataDir, Weaker: weakerOK, Net: mode, NetBestEffort: best, OpenFiles: spec.Limits.openFiles()}, proxy: spec.Proxy}
	if spec.SelfAddr != "" {
		ap, err := netip.ParseAddrPort(spec.SelfAddr)
		if err != nil {
			return fmt.Errorf("plugin app address: %w", err)
		}
		st.self = netip.AddrPortFrom(ap.Addr().Unmap(), ap.Port())
	}
	helper := s.Helper
	if helper == "" {
		self, err := os.Executable()
		if err != nil {
			return fmt.Errorf("plugin sandbox helper: %w", err)
		}
		helper = self
	}
	env := cmd.Env[:0:0]
	for _, kv := range cmd.Env {
		if !strings.HasPrefix(kv, "GO") {
			env = append(env, kv)
		}
	}
	cmd.Env = env
	cmd.Path = helper
	if mode == netNotify {
		fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
		if err != nil {
			return fmt.Errorf("plugin sandbox socketpair: %w", err)
		}
		st.parent, st.child = os.NewFile(uintptr(fds[0]), "sandbox-parent"), os.NewFile(uintptr(fds[1]), "sandbox-child")
		cmd.ExtraFiles = append(cmd.ExtraFiles, st.child) // fd 3
	}
	if landlockABI() >= 1 {
		for _, f := range spec.Folders {
			if f.File == nil {
				continue
			}
			cmd.ExtraFiles = append(cmd.ExtraFiles, f.File)
			st.cfg.Folders = append(st.cfg.Folders, helperFolder{FD: 2 + len(cmd.ExtraFiles), Write: f.Mode == FolderReadWrite})
		}
	}
	if cgroupUsable() {
		if st.cfg.Cgroup, err = cgroupCreate(spec.ID, spec.Limits); err != nil {
			st.closeFiles()
			return err
		}
	}
	raw, err := json.Marshal(st.cfg)
	if err != nil {
		st.closeFiles()
		return err
	}
	cmd.Args = append([]string{helper, sandboxHelperArg, string(raw), spec.Program}, spec.Args...)
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	var once sync.Once
	var cancelErr error
	cg := st.cfg.Cgroup
	cmd.Cancel = func() error {
		once.Do(func() {
			cancelErr = killProcess(cmd)
			if cg != "" {
				cancelErr = cgroupKill(cg)
			}
		})
		return cancelErr
	}
	sbStates.Store(cmd, st)
	return nil
}

func (st *sbState) closeFiles() {
	if st.parent != nil {
		st.parent.Close()
		st.parent = nil
	}
	if st.child != nil {
		st.child.Close()
		st.child = nil
	}
}

// Started runs right after the helper started: it takes the seccomp
// listener the helper sends and starts the supervisor.
func (LinuxSandbox) Started(cmd *exec.Cmd) error {
	v, ok := sbStates.Load(cmd)
	if !ok {
		return nil
	}
	st := v.(*sbState)
	if st.child != nil {
		st.child.Close()
		st.child = nil
	}
	if st.parent == nil {
		return nil
	}
	parent := st.parent
	st.parent = nil
	defer parent.Close()
	fd, err := recvFD(parent, 10*time.Second)
	if err != nil {
		return fmt.Errorf("plugin network lock: %w", err)
	}
	st.sup = startSupervisor(fd, st.self, st.proxy)
	return nil
}

// recvFD reads one file descriptor sent with SCM_RIGHTS.
func recvFD(f *os.File, wait time.Duration) (int, error) {
	c, err := net.FileConn(f)
	if err != nil {
		return -1, err
	}
	defer c.Close()
	uc, ok := c.(*net.UnixConn)
	if !ok {
		return -1, errors.New("not a unix socket")
	}
	_ = uc.SetReadDeadline(time.Now().Add(wait))
	buf := make([]byte, 1)
	oob := make([]byte, unix.CmsgSpace(4))
	_, oobn, _, _, err := uc.ReadMsgUnix(buf, oob)
	none := errors.New("the plugin helper sent no seccomp listener")
	if err != nil {
		return -1, fmt.Errorf("%w: %v", none, err)
	}
	msgs, err := unix.ParseSocketControlMessage(oob[:oobn])
	if err != nil || len(msgs) != 1 {
		return -1, none
	}
	fds, err := unix.ParseUnixRights(&msgs[0])
	if err != nil || len(fds) != 1 {
		for _, fd := range fds {
			unix.Close(fd)
		}
		return -1, none
	}
	return fds[0], nil
}

// Cleanup removes a start's cgroup and supervisor when Start failed or the
// program ended, killing anything it left behind.
func (LinuxSandbox) Cleanup(cmd *exec.Cmd) error {
	v, ok := sbStates.LoadAndDelete(cmd)
	if !ok {
		return nil
	}
	st := v.(*sbState)
	st.closeFiles()
	var err error
	if st.cfg.Cgroup != "" {
		err = cgroupKill(st.cfg.Cgroup)
	}
	if st.sup != nil {
		st.sup.Close()
	}
	return err
}

// Active reports which resource caps, locks and stop guarantees this host gives.
func (s LinuxSandbox) Active() ActiveLimits {
	al := cgroupActive()
	al.FileLock = landlockABI() >= 1
	al.NetLock = s.NetLock
	return al
}

// RunSandboxHelper does nothing unless this process was started as the
// sandbox helper or the network lock probe; then it locks down and execs
// the plugin, never returning.
func RunSandboxHelper() {
	if len(os.Args) >= 2 && os.Args[1] == netProbeArg {
		os.Exit(netProbeChild())
	}
	if len(os.Args) < 4 || os.Args[1] != sandboxHelperArg {
		return
	}
	var cfg helperConfig
	err := json.Unmarshal([]byte(os.Args[2]), &cfg)
	if err == nil {
		err = sandboxExec(cfg, os.Args[3], os.Args[4:])
	}
	fmt.Fprintln(os.Stderr, "plugin sandbox:", err)
	os.Exit(126)
}

func sandboxExec(cfg helperConfig, program string, args []string) error {
	if cfg.Cgroup != "" {
		if err := cgroupJoin(cfg.Cgroup); err != nil {
			return fmt.Errorf("join cgroup: %w", err)
		}
	}
	runtime.LockOSThread() // Landlock, seccomp and no_new_privs are per thread; exec from this one.
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("no_new_privs: %w", err)
	}
	// syscall.Setrlimit (not unix) so Go does not put back its raised file limit on exec.
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &syscall.Rlimit{Cur: max(cfg.OpenFiles, 64), Max: max(cfg.OpenFiles, 64)}); err != nil {
		return fmt.Errorf("rlimit open files: %w", err)
	}
	if err := syscall.Setrlimit(syscall.RLIMIT_CORE, &syscall.Rlimit{}); err != nil {
		return fmt.Errorf("rlimit core: %w", err)
	}
	if err := landlockRestrict(cfg.DataDir, program, cfg.Folders); err != nil {
		if !errors.Is(err, ErrNoLandlock) || !cfg.Weaker {
			return err
		}
	}
	for _, f := range cfg.Folders {
		unix.Close(f.FD)
	}
	listener, err := installFilter(cfg.Net)
	if err != nil && !cfg.NetBestEffort {
		return err
	}
	if listener >= 0 {
		if err := unix.Sendmsg(3, []byte{1}, unix.UnixRights(listener), nil, 0); err != nil {
			return fmt.Errorf("send seccomp listener: %w", err)
		}
		unix.Close(listener)
		unix.Close(3)
	}
	return unix.Exec(program, append([]string{program}, args...), os.Environ())
}

// landlockABI returns the kernel's Landlock ABI version, 0 when it has none.
func landlockABI() int {
	abi, _ := landlockProbe()
	return abi
}

// landlockProbe returns the Landlock ABI version, or 0 and the errno that
// says why it is missing.
func landlockProbe() (int, syscall.Errno) {
	v, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0, unix.LANDLOCK_CREATE_RULESET_VERSION)
	if errno != 0 {
		return 0, errno
	}
	return int(v), 0
}

// fileLockReason maps the probe's errno to a reason; unknown errors count as
// an old kernel, the case with no setting to change.
func fileLockReason(errno syscall.Errno) string {
	switch errno {
	case unix.EPERM:
		return FileLockBlocked
	case unix.EOPNOTSUPP:
		return FileLockTurnedOff
	}
	return FileLockOldKernel
}

const (
	llFileRead = unix.LANDLOCK_ACCESS_FS_READ_FILE
	llDirRead  = unix.LANDLOCK_ACCESS_FS_READ_FILE | unix.LANDLOCK_ACCESS_FS_READ_DIR
	llFileRW   = unix.LANDLOCK_ACCESS_FS_READ_FILE | unix.LANDLOCK_ACCESS_FS_WRITE_FILE | unix.LANDLOCK_ACCESS_FS_TRUNCATE
	llDirRW    = llFileRW | unix.LANDLOCK_ACCESS_FS_READ_DIR | unix.LANDLOCK_ACCESS_FS_REMOVE_DIR |
		unix.LANDLOCK_ACCESS_FS_REMOVE_FILE | unix.LANDLOCK_ACCESS_FS_MAKE_DIR | unix.LANDLOCK_ACCESS_FS_MAKE_REG |
		unix.LANDLOCK_ACCESS_FS_MAKE_SYM | unix.LANDLOCK_ACCESS_FS_REFER
	// llFolderRW is an approved read-write folder: no symlinks (nor fifos,
	// sockets or devices, which llDirRW never had), so a plugin cannot plant
	// links for HolyArrs's own scanners.
	llFolderRW = llDirRW &^ unix.LANDLOCK_ACCESS_FS_MAKE_SYM
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

func landlockRestrict(dataDir, program string, folders []helperFolder) error {
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
	for _, f := range folders {
		access := uint64(llDirRead)
		if f.Write {
			access = llFolderRW
		}
		rule := unix.LandlockPathBeneathAttr{Allowed_access: access & handled, Parent_fd: int32(f.FD)}
		if _, _, errno := unix.Syscall6(unix.SYS_LANDLOCK_ADD_RULE, fd, landlockRulePathBeneath, uintptr(unsafe.Pointer(&rule)), 0, 0, 0); errno != 0 {
			return fmt.Errorf("landlock folder: %w", errno)
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
	sb, al := defaultSandbox(landlockProbe())
	sb.NetLock, al.NetLockReason = netLockProbe()
	al.NetLock = sb.NetLock
	return ExecRunner{Sandbox: sb}, al, nil
}

// defaultSandbox picks the sandbox for the Landlock probe result. It never
// allows a weaker start on its own: without Landlock a plugin starts only
// when the owner allowed it for that plugin.
func defaultSandbox(abi int, errno syscall.Errno) (LinuxSandbox, ActiveLimits) {
	al := cgroupActive()
	al.FileLock = abi >= 1
	if !al.FileLock {
		al.FileLockReason = fileLockReason(errno)
	}
	return LinuxSandbox{}, al
}
