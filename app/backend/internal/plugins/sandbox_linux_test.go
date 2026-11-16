//go:build linux

package plugins

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestMain lets the test binary act as the sandbox helper.
func TestMain(m *testing.M) {
	RunSandboxHelper()
	os.Exit(m.Run())
}

func TestLinuxSandboxLimitsFiles(t *testing.T) {
	if landlockABI() < 1 {
		t.Skip("kernel has no Landlock; sandbox test skipped")
	}
	root := t.TempDir()
	data := filepath.Join(root, "data")
	ver := filepath.Join(root, "ver")
	for _, d := range []string{data, ver} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	probe := filepath.Join(ver, "run-probe")
	build := exec.Command("go", "build", "-o", probe, "./testdata/sandboxprobe")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build probe: %v\n%s", err, out)
	}
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
