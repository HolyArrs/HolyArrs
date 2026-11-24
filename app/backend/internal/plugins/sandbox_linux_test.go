//go:build linux

package plugins

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// TestMain lets the test binary act as the sandbox helper.
func TestMain(m *testing.M) {
	RunSandboxHelper()
	os.Exit(m.Run())
}

// probeDirs builds the probe into root/ver and returns the data folder and probe path.
func probeDirs(t *testing.T) (root, data, probe string) {
	t.Helper()
	root = t.TempDir()
	data = filepath.Join(root, "data")
	ver := filepath.Join(root, "ver")
	for _, d := range []string{data, ver} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	probe = filepath.Join(ver, "run-probe")
	if pre := os.Getenv("HOLYARRS_PROBE"); pre != "" { // a prebuilt probe, for images without Go
		b, err := os.ReadFile(pre)
		if err != nil || os.WriteFile(probe, b, 0o700) != nil {
			t.Fatalf("copy prebuilt probe: %v", err)
		}
		return root, data, probe
	}
	build := exec.Command("go", "build", "-o", probe, "./testdata/sandboxprobe")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build probe: %v\n%s", err, out)
	}
	return root, data, probe
}

func TestLinuxSandboxLimitsFiles(t *testing.T) {
	if landlockABI() < 1 {
		t.Skip("kernel has no Landlock; sandbox test skipped")
	}
	root, data, probe := probeDirs(t)
	ver := filepath.Dir(probe)
	secret := filepath.Join(root, "secret.txt")
	if err := os.WriteFile(secret, []byte("s"), 0o600); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) error {
		p, err := (ExecRunner{Sandbox: LinuxSandbox{}}).Start(context.Background(), Spec{Program: probe, Args: args, DataDir: data})
		if err != nil {
			t.Fatal(err)
		}
		return p.Wait()
	}
	if err := run("write", filepath.Join(data, "ok.txt")); err != nil {
		t.Errorf("write in data folder failed: %v", err)
	}
	if err := run("read", secret); err == nil {
		t.Error("read outside allowed paths worked")
	}
	if err := run("write", filepath.Join(ver, "next.txt")); err == nil {
		t.Error("write next to run copy worked")
	}
	if err := run("read", "/etc/hosts"); err != nil {
		t.Errorf("read /etc/hosts failed: %v", err)
	}
}

func TestLinuxSandboxBlocksSignalToParent(t *testing.T) {
	if landlockABI() < 6 {
		t.Skip("kernel Landlock ABI below 6; signal scope test skipped")
	}
	_, data, probe := probeDirs(t)
	p, err := (ExecRunner{Sandbox: LinuxSandbox{}}).Start(context.Background(), Spec{Program: probe, Args: []string{"signalparent"}, DataDir: data})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Wait(); err == nil {
		t.Error("probe could signal its parent")
	}
}

func TestLinuxSandboxStopKillsLeftChildren(t *testing.T) {
	if landlockABI() < 1 {
		t.Skip("kernel has no Landlock; sandbox test skipped")
	}
	_, data, probe := probeDirs(t)
	pidFile := filepath.Join(data, "child.pid")
	p, err := (ExecRunner{Sandbox: LinuxSandbox{}}).Start(context.Background(), Spec{Program: probe, Args: []string{"fork", pidFile}, DataDir: data})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Wait(); err != nil {
		t.Fatalf("probe fork: %v", err)
	}
	b, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := strconv.Atoi(string(b))
	if err := syscall.Kill(pid, 0); err != nil {
		t.Fatalf("child not running before stop: %v", err)
	}
	if err := p.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	// The child is not ours to reap; it is gone once it is a zombie or reaped.
	for i := 0; i < 100; i++ {
		st, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
		if errors.Is(err, os.ErrNotExist) || (err == nil && statState(st) == 'Z') {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	syscall.Kill(pid, syscall.SIGKILL)
	t.Error("child still running after Stop")
}

func requireCgroup(t *testing.T) {
	t.Helper()
	if landlockABI() < 1 {
		t.Skip("kernel has no Landlock; sandbox test skipped")
	}
	if !(LinuxSandbox{}).Active().Cgroup {
		t.Skip("no delegated, writable cgroup v2 with pids, memory and cpu; cgroup test skipped")
	}
}

func TestLinuxSandboxCgroupCapsPids(t *testing.T) {
	requireCgroup(t)
	_, data, probe := probeDirs(t)
	p, err := (ExecRunner{Sandbox: LinuxSandbox{}}).Start(context.Background(), Spec{ID: "pids", Program: probe, Args: []string{"forkmany", "400"}, DataDir: data})
	if err != nil {
		t.Fatal(err)
	}
	werr := p.Wait()
	if err := p.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if werr != nil {
		t.Errorf("probe forked past pids.max: %v", werr)
	}
}

func TestLinuxSandboxCgroupKillsEscapedChild(t *testing.T) {
	requireCgroup(t)
	_, data, probe := probeDirs(t)
	pidFile := filepath.Join(data, "child.pid")
	p, err := (ExecRunner{Sandbox: LinuxSandbox{}}).Start(context.Background(), Spec{ID: "escape", Program: probe, Args: []string{"escape", pidFile}, DataDir: data})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Wait(); err != nil {
		t.Fatalf("probe escape: %v", err)
	}
	b, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := strconv.Atoi(string(b))
	if err := p.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	for i := 0; i < 100; i++ {
		st, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
		if errors.Is(err, os.ErrNotExist) || (err == nil && statState(st) == 'Z') {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	syscall.Kill(pid, syscall.SIGKILL)
	t.Error("setsid child still running after Stop")
}

func TestLinuxSandboxActiveReportsGroupKill(t *testing.T) {
	a := (LinuxSandbox{}).Active()
	if !a.GroupKill {
		t.Error("group kill not reported")
	}
	t.Logf("active limits: %+v", a)
}

func statState(st []byte) byte {
	for i := len(st) - 1; i > 0; i-- {
		if st[i] == ')' && i+2 < len(st) {
			return st[i+2]
		}
	}
	return 0
}

// TestServerBinaryRunsSandboxHelper builds the real server and checks that in
// helper mode it execs the plugin instead of starting the server.
func TestServerBinaryRunsSandboxHelper(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the whole server")
	}
	root, data, probe := probeDirs(t)
	server := filepath.Join(root, "holyarrs")
	build := exec.Command("go", "build", "-o", server, "../../cmd/holyarrs")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build server: %v\n%s", err, out)
	}
	ok := filepath.Join(data, "ok.txt")
	cfg := `{"data":"` + data + `","weaker":true,"net":"none"}`
	cmd := exec.Command(server, sandboxHelperArg, cfg, probe, "write", ok)
	cmd.Env = []string{}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("helper mode: %v\n%s", err, out)
	}
	if _, err := os.Stat(ok); err != nil {
		t.Fatalf("probe did not run: %v", err)
	}
}

// TestLinuxSandboxCannotWriteCgroupLimits checks that a plugin cannot raise
// the caps of a plugin cgroup.
func TestLinuxSandboxCannotWriteCgroupLimits(t *testing.T) {
	requireCgroup(t)
	_, data, probe := probeDirs(t)
	r := ExecRunner{Sandbox: LinuxSandbox{}}
	sleeper, err := r.Start(context.Background(), Spec{ID: "cgmem", Program: probe, Args: []string{"sleep"}, DataDir: data})
	if err != nil {
		t.Fatal(err)
	}
	defer sleeper.Stop()
	dirs, _ := filepath.Glob(filepath.Join(cgroupBase(), "holyarrs-plugin-cgmem-*"))
	if len(dirs) != 1 {
		t.Fatalf("plugin cgroups: %v", dirs)
	}
	file := filepath.Join(dirs[0], "memory.max")
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	p, err := r.Start(context.Background(), Spec{ID: "cgmem-writer", Program: probe, Args: []string{"write", file}, DataDir: data})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Wait(); err == nil {
		t.Error("probe wrote memory.max of a plugin cgroup")
	}
	if after, _ := os.ReadFile(file); string(after) != string(before) {
		t.Errorf("memory.max changed: %q -> %q", before, after)
	}
}

// Without Landlock the default sandbox fails closed: it never allows a
// weaker start on its own, keeps the cgroup caps, and says why the lock is
// missing so the page can say what to fix.
func TestDefaultSandboxWithoutLandlock(t *testing.T) {
	for _, c := range []struct {
		errno syscall.Errno
		want  string
	}{{unix.ENOSYS, FileLockOldKernel}, {unix.EPERM, FileLockBlocked}, {unix.EOPNOTSUPP, FileLockTurnedOff}, {unix.EINVAL, FileLockOldKernel}} {
		sb, al := defaultSandbox(0, c.errno)
		if sb.AllowWeaker || al.FileLock || al.FileLockReason != c.want {
			t.Fatalf("%v: AllowWeaker=%v FileLock=%v reason=%q, want false/false/%q", c.errno, sb.AllowWeaker, al.FileLock, al.FileLockReason, c.want)
		}
		if want := cgroupActive(); al.Memory != want.Memory || al.Pids != want.Pids || al.GroupKill != want.GroupKill {
			t.Fatalf("caps changed without Landlock: %+v, want %+v", al, want)
		}
	}
	sb, al := defaultSandbox(3, 0)
	if sb.AllowWeaker || !al.FileLock || al.FileLockReason != "" {
		t.Fatalf("Landlock 3: %+v %+v", sb, al)
	}
}

// Apply refuses a start without Landlock unless that start carries the
// owner's opt-in; with Landlock the opt-in changes nothing.
func TestApplyNeedsOptInWithoutLandlock(t *testing.T) {
	spec := Spec{ID: "optin", Program: "/bin/true", DataDir: t.TempDir()}
	cmd := exec.Command(spec.Program)
	err := LinuxSandbox{Helper: "/bin/true"}.Apply(cmd, spec)
	if landlockABI() >= 1 {
		if err != nil {
			t.Fatalf("with Landlock: %v", err)
		}
		if strings.Contains(cmd.Args[2], `"weaker"`) {
			t.Fatalf("weaker flag set without opt-in: %v", cmd.Args)
		}
		LinuxSandbox{}.Cleanup(cmd)
		return
	}
	if !errors.Is(err, ErrNoLandlock) {
		t.Fatalf("no Landlock, no opt-in: %v, want ErrNoLandlock", err)
	}
	spec.AllowNoFileLock = true
	cmd = exec.Command(spec.Program)
	if err := (LinuxSandbox{Helper: "/bin/true"}).Apply(cmd, spec); err != nil {
		t.Fatalf("no Landlock, opt-in: %v", err)
	}
	if !strings.Contains(cmd.Args[2], `"weaker":true`) {
		t.Fatalf("weaker flag not passed to the helper: %v", cmd.Args)
	}
	LinuxSandbox{}.Cleanup(cmd)
}
