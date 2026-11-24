package plugins

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// ErrNoLandlock means the system cannot apply the filesystem lockdown and
// the owner has not allowed this plugin to start without it.
var ErrNoLandlock = errors.New("plugin sandbox: the file lock (Landlock) is not available on this system")

// ErrNoNetLock means the system cannot keep a plugin to the websites it
// lists and the owner has not allowed plugins to start without that.
var ErrNoNetLock = errors.New("plugin sandbox: the network lock (seccomp user-notify) is not available on this system")

// Why the file lock or the network lock is missing, as the page shows it.
const (
	FileLockOldKernel = "old_kernel" // ENOSYS: the kernel has no Landlock
	FileLockBlocked   = "blocked"    // EPERM: a container seccomp or security profile blocks it
	FileLockTurnedOff = "turned_off" // EOPNOTSUPP: built in but turned off at boot (lsm=)
)

// Spec is one program start. Env is the whole environment: nothing from the
// app's own environment is passed on, and DataDir is the only folder given.
type Spec struct {
	// ID names the plugin; it names its cgroup.
	ID      string
	Program string
	Args    []string
	Env     []string
	DataDir string
	// Limits overrides the default resource caps; zero fields keep the defaults.
	Limits Limits
	// AllowNoFileLock lets this start go ahead without Landlock when the
	// system cannot give it; the owner set it for this plugin.
	AllowNoFileLock bool
	// Proxy is the plugin's way out to its listed hosts; nil when it lists
	// none. SelfAddr is the app's own address ("127.0.0.1:6262") when the
	// plugin is handed it.
	Proxy    *ProxyHandle
	SelfAddr string
	// AllowNoNetLock lets a plugin with hosts start when the system has no
	// network lock; the owner chose it for the whole install.
	AllowNoNetLock bool
	// Folders are the folders the owner approved, already open.
	Folders []GrantedFolder
}

// GrantedFolder is one approved folder for one start. File is the folder
// opened by HolyArrs (O_PATH); the sandbox locks to that open file.
type GrantedFolder struct {
	Name string
	Path string
	Mode string
	File *os.File
}

// Limits are resource caps for one start.
type Limits struct {
	// MemoryMax is in bytes; 0 means the default (512 MiB). CPUs,
	// Processes and OpenFiles: 0 means the default (1, 256, 1024).
	MemoryMax int64
	CPUs      int
	Processes int
	OpenFiles int
}

// ActiveLimits says which caps a sandbox can actually apply, so the UI can
// show it.
type ActiveLimits struct {
	Cgroup    bool // the plugin runs in its own cgroup, killed as a whole on stop
	Pids      bool // pids.max 256
	Memory    bool // memory.max
	CPU       bool // cpu.max, 1 CPU
	GroupKill bool // its own process group is killed on stop
	FileLock  bool // Landlock limits it to its own folders; false on kernels without it
	// FileLockReason says why FileLock is off: FileLockOldKernel,
	// FileLockBlocked or FileLockTurnedOff. Empty when it is on.
	FileLockReason string
	// NetLock is true when plugins can be kept to the websites they list
	// (seccomp user-notify); NetLockReason says why not, like FileLockReason.
	NetLock       bool
	NetLockReason string
}

// Runner starts plugin programs.
type Runner interface {
	Start(ctx context.Context, spec Spec) (Process, error)
}

// Process is one running plugin program.
type Process interface {
	// Stop ends the program and waits for it.
	Stop() error
	// Wait blocks until the program ends.
	Wait() error
}

// Sandbox locks a program down before it starts: its own OS user and only
// the hosts, folders and actions it declared. It is the hook for the
// OS-level lockdown; NoSandbox applies nothing.
type Sandbox interface {
	Apply(cmd *exec.Cmd, spec Spec) error
}

// NoSandbox applies no lockdown.
type NoSandbox struct{}

// Apply does nothing.
func (NoSandbox) Apply(*exec.Cmd, Spec) error { return nil }

// ExecRunner starts real programs.
type ExecRunner struct {
	Sandbox Sandbox
}

// Start runs the program in its data folder with only the given env plus
// the data folder's path as HOLYARRS_PLUGIN_DATA.
func (r ExecRunner) Start(ctx context.Context, spec Spec) (Process, error) {
	if !filepath.IsAbs(spec.Program) {
		return nil, errors.New("plugin program path is not absolute")
	}
	cmd := exec.CommandContext(ctx, spec.Program, spec.Args...)
	cmd.Dir = spec.DataDir
	cmd.Env = append(append([]string{}, spec.Env...), "HOLYARRS_PLUGIN_DATA="+spec.DataDir)
	sb := r.Sandbox
	if sb == nil {
		sb = NoSandbox{}
	}
	if err := sb.Apply(cmd, spec); err != nil {
		return nil, err
	}
	cleanup := func() error { return nil }
	if c, ok := sb.(interface{ Cleanup(*exec.Cmd) error }); ok {
		cleanup = func() error { return c.Cleanup(cmd) }
	}
	// The program writes straight into an OS pipe, so Wait only waits for the
	// program itself; a child it left keeps logging until Stop kills it.
	out, err := pluginLog(spec.ID)
	if err != nil {
		_ = cleanup()
		return nil, err
	}
	cmd.Stdout, cmd.Stderr = out, out
	err = cmd.Start()
	_ = out.Close() // the program holds its own copy
	if err != nil {
		_ = cleanup()
		return nil, err
	}
	if s, ok := sb.(interface{ Started(*exec.Cmd) error }); ok {
		if err := s.Started(cmd); err != nil {
			_ = killProcess(cmd)
			_ = cmd.Wait()
			_ = cleanup()
			return nil, err
		}
	}
	p := &execProcess{cmd: cmd, done: make(chan struct{})}
	if a, ok := sb.(interface{ Active() ActiveLimits }); ok {
		p.limits = a.Active()
	}
	go func() {
		p.err = cmd.Wait()
		p.cleanupErr = cleanup()
		close(p.done)
	}()
	return p, nil
}

type execProcess struct {
	cmd  *exec.Cmd
	done chan struct{}
	err  error
	once sync.Once
	// limits are the caps this start actually got.
	limits     ActiveLimits
	cleanupErr error
	stopErr    error
}

// Limits reports the caps this start actually got.
func (p *execProcess) Limits() ActiveLimits { return p.limits }

func (p *execProcess) Stop() error {
	p.once.Do(func() {
		if p.cmd.Cancel != nil {
			p.stopErr = p.cmd.Cancel()
		} else {
			p.stopErr = killProcess(p.cmd)
		}
	})
	<-p.done
	var exit *exec.ExitError
	if p.err != nil && !errors.As(p.err, &exit) {
		return p.err
	}
	if p.cleanupErr != nil {
		return p.cleanupErr
	}
	if p.stopErr != nil && !errors.Is(p.stopErr, os.ErrProcessDone) && !errors.Is(p.stopErr, syscall.ESRCH) {
		return p.stopErr
	}
	return nil
}

func (p *execProcess) Wait() error {
	<-p.done
	return p.err
}

// pluginLog copies a plugin's output, line by line, into the HolyArrs log as
// "plugin <id>: <line>". Lines over 64 KiB are cut. The reader ends when
// every process holding the write end has exited.
func pluginLog(id string) (*os.File, error) {
	if id == "" {
		id = "?"
	}
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	go func() {
		defer r.Close()
		logLines(id, r, time.Now)
	}()
	return w, nil
}

// Plugin output limits: a line longer than maxLogLine is cut (the rest of it
// skipped, later lines still logged), and at most logBurst lines are logged
// per logEvery; the lines over that are counted and reported once.
const (
	maxLogLine = 64 << 10
	logBurst   = 200
	logEvery   = 10 * time.Second
)

func logLines(id string, r io.Reader, now func() time.Time) {
	br := bufio.NewReaderSize(r, 4096)
	var line []byte
	cut := false
	var start time.Time
	n, dropped := 0, 0
	emit := func(text string) {
		if t := now(); t.Sub(start) >= logEvery {
			if dropped > 0 {
				log.Printf("plugin %s: (%d more lines not logged: too much output)", id, dropped)
			}
			start, n, dropped = t, 0, 0
		}
		if n >= logBurst {
			dropped++
			return
		}
		n++
		log.Printf("plugin %s: %s", id, text)
	}
	for {
		chunk, isPrefix, err := br.ReadLine()
		if len(chunk) > 0 && !cut {
			room := maxLogLine - len(line)
			if len(chunk) > room {
				chunk, cut = chunk[:room], true
			}
			line = append(line, chunk...)
		}
		if err != nil {
			if len(line) > 0 {
				emit(string(line))
			}
			if dropped > 0 {
				log.Printf("plugin %s: (%d more lines not logged: too much output)", id, dropped)
			}
			return
		}
		if isPrefix {
			continue
		}
		text := string(line)
		if cut {
			text += " [line cut]"
		}
		emit(text)
		line, cut = line[:0], false
	}
}
