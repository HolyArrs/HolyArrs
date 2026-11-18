package plugins

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
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
}

// Limits are resource caps for one start.
type Limits struct {
	// MemoryMax is in bytes; 0 means the default (512 MiB).
	MemoryMax int64
}

// ActiveLimits says which caps a sandbox can actually apply, so the UI can
// show it.
type ActiveLimits struct {
	Cgroup    bool // the plugin runs in its own cgroup, killed as a whole on stop
	Pids      bool // pids.max 256
	Memory    bool // memory.max
	CPU       bool // cpu.max, 1 CPU
	GroupKill bool // its own process group is killed on stop
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
	if err := cmd.Start(); err != nil {
		_ = cleanup()
		return nil, err
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
