package plugins

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type fixture struct {
	t     *testing.T
	store *Store
	pub   []byte
	sign  func(kind string, counter uint64, entries ...Entry) []byte
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pub, priv := testKey(t)
	st, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, store: st, pub: pub}
	f.sign = func(kind string, counter uint64, entries ...Entry) []byte {
		l := List{Kind: kind, Counter: counter, Entries: entries}
		if kind == KindPlugins {
			l.MinBlocklistCounter = u64(0)
		}
		if l.Entries == nil {
			l.Entries = []Entry{}
		}
		for i := range l.Entries {
			if kind == KindPlugins && l.Entries[i].ManifestSHA256 == "" {
				l.Entries[i].ManifestSHA256 = fileHash([]byte("start checks never read the manifest"))
			}
		}
		return sign(t, priv, l)
	}
	return f
}

// install runs CheckInstall and Install for one version of the program.
func (f *fixture) install(version string, file []byte) error {
	f.t.Helper()
	return f.store.Install(f.accepted(version, file))
}

// accepted runs CheckInstall for one version of the program.
func (f *fixture) accepted(version string, file []byte) *Accepted {
	man := manifestJSON(func(m map[string]any) { m["version"] = version; m["sha256"] = fileHash(file) })
	acc, err := CheckInstall(InstallInput{
		PublicKey: f.pub,
		List:      f.sign(KindPlugins, 1, Entry{ID: "tailscale", Version: version, SHA256: fileHash(file), ManifestSHA256: fileHash(man)}),
		Blocklist: f.sign(KindBlocklist, 1),
		Manifest:  man,
		File:      strings.NewReader(string(file)),
		TempDir:   f.t.TempDir(),
	})
	if err != nil {
		f.t.Error(err)
	}
	return acc
}

func (f *fixture) startInput(version string, file []byte, block ...Entry) StartInput {
	return StartInput{
		PublicKey: f.pub,
		List:      f.sign(KindPlugins, 1, Entry{ID: "tailscale", Version: version, SHA256: fileHash(file)}),
		Blocklist: f.sign(KindBlocklist, 1, block...),
	}
}

func (f *fixture) state() *State {
	f.t.Helper()
	st, err := f.store.Load()
	if err != nil {
		f.t.Fatal(err)
	}
	return st
}

func (f *fixture) programPath() string {
	f.t.Helper()
	return f.store.ProgramPath(f.state().Plugins["tailscale"])
}

// recordRunner records starts; pluginstest.FakeRunner would be an import
// cycle here.
type recordRunner struct {
	mu     sync.Mutex
	Starts []Spec
}

func (r *recordRunner) Start(_ context.Context, spec Spec) (Process, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Starts = append(r.Starts, spec)
	return stoppedProcess{}, nil
}

type stoppedProcess struct{}

func (stoppedProcess) Stop() error { return nil }
func (stoppedProcess) Wait() error { return nil }

func TestInstallAndStart(t *testing.T) {
	f := newFixture(t)
	if err := f.install("1.0.0", programFile); err != nil {
		t.Fatal(err)
	}
	p := f.state().Plugins["tailscale"]
	if p == nil || !p.Enabled || p.Version != "1.0.0" || p.SHA256 != fileHash(programFile) {
		t.Fatalf("state: %+v", p)
	}
	r := &recordRunner{}
	proc, err := f.store.Start(context.Background(), r, "tailscale", f.startInput("1.0.0", programFile), []string{"up"}, []string{"A=1", "HOLYARRS_PLUGIN_X=1"})
	if err != nil {
		t.Fatal(err)
	}
	proc.Stop()
	prog := f.programPath()
	if len(r.Starts) != 1 || r.Starts[0].DataDir != f.store.DataDir("tailscale") ||
		filepath.Dir(r.Starts[0].Program) != filepath.Dir(prog) ||
		!strings.HasPrefix(filepath.Base(r.Starts[0].Program), "run-") {
		t.Fatalf("starts: %+v", r.Starts)
	}
	if got, err := hashFile(r.Starts[0].Program); err != nil || got != fileHash(programFile) {
		t.Fatalf("run copy: %s %v", got, err)
	}
	env := strings.Join(r.Starts[0].Env, "\n")
	if strings.Contains(env, "A=1") || !strings.Contains(env, "HOLYARRS_PLUGIN_X=1") ||
		!strings.Contains(env, "HOME="+f.store.DataDir("tailscale")) {
		t.Fatalf("env: %v", r.Starts[0].Env)
	}
	if fi, err := os.Stat(r.Starts[0].DataDir); err != nil || !fi.IsDir() {
		t.Fatalf("data folder: %v", err)
	}
}

func TestUpdateReplacesOldVersion(t *testing.T) {
	f := newFixture(t)
	newFile := []byte("version two")
	if err := f.install("1.0.0", programFile); err != nil {
		t.Fatal(err)
	}
	oldDir := filepath.Dir(f.programPath())
	if err := f.install("1.1.0", newFile); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Fatalf("old version folder still there: %v", err)
	}
	if _, err := f.store.CheckStart("tailscale", f.startInput("1.1.0", newFile)); err != nil {
		t.Fatal(err)
	}
}

func TestStartRefusesTamperedProgram(t *testing.T) {
	f := newFixture(t)
	if err := f.install("1.0.0", programFile); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.programPath(), []byte("evil"), 0o700); err != nil {
		t.Fatal(err)
	}
	r := &recordRunner{}
	if _, err := f.store.Start(context.Background(), r, "tailscale", f.startInput("1.0.0", programFile), nil, nil); !errors.Is(err, ErrTampered) {
		t.Fatalf("got %v, want ErrTampered", err)
	}
	if len(r.Starts) != 0 {
		t.Fatal("tampered program was started")
	}
	p := f.state().Plugins["tailscale"]
	if p.Enabled || !strings.Contains(p.DisabledReason, "does not match") {
		t.Fatalf("not disabled with reason: %+v", p)
	}
	if _, err := f.store.CheckStart("tailscale", f.startInput("1.0.0", programFile)); !errors.Is(err, ErrDisabled) {
		t.Fatalf("second start: %v", err)
	}
}

func TestStartRefusesMissingProgram(t *testing.T) {
	f := newFixture(t)
	if err := f.install("1.0.0", programFile); err != nil {
		t.Fatal(err)
	}
	os.Remove(f.programPath())
	if _, err := f.store.CheckStart("tailscale", f.startInput("1.0.0", programFile)); !errors.Is(err, ErrTampered) {
		t.Fatalf("got %v", err)
	}
}

func TestStartRefusesBlocklistedAfterInstall(t *testing.T) {
	for _, e := range []Entry{{ID: "tailscale"}, {ID: "tailscale", Version: "1.0.0"}, {SHA256: fileHash(programFile)}} {
		f := newFixture(t)
		if err := f.install("1.0.0", programFile); err != nil {
			t.Fatal(err)
		}
		if _, err := f.store.CheckStart("tailscale", f.startInput("1.0.0", programFile, e)); !errors.Is(err, ErrBlocked) {
			t.Fatalf("%+v: got %v", e, err)
		}
		if p := f.state().Plugins["tailscale"]; p.Enabled || !strings.Contains(p.DisabledReason, "blocklist") {
			t.Fatalf("%+v: not disabled: %+v", e, p)
		}
	}
}

func TestStartRefusesListMismatches(t *testing.T) {
	f := newFixture(t)
	if err := f.install("1.0.0", programFile); err != nil {
		t.Fatal(err)
	}
	in := f.startInput("1.0.0", programFile)
	in.List = f.sign(KindPlugins, 1, Entry{ID: "tailscale", Version: "1.0.0", SHA256: fileHash([]byte("other"))})
	if _, err := f.store.CheckStart("tailscale", in); !errors.Is(err, ErrNotOnList) {
		t.Fatalf("got %v", err)
	}

	f = newFixture(t)
	if err := f.install("1.0.0", programFile); err != nil {
		t.Fatal(err)
	}
	in = f.startInput("1.0.0", programFile)
	in.List = f.sign(KindPlugins, 0, Entry{ID: "tailscale", Version: "1.0.0", SHA256: fileHash(programFile)})
	if _, err := f.store.CheckStart("tailscale", in); !errors.Is(err, ErrListOlder) {
		t.Fatalf("older list: %v", err)
	}
}

func TestCorruptStateRefused(t *testing.T) {
	for name, body := range map[string]string{
		"not json":      "{",
		"unknown field": `{"list_counter":1,"extra":true}`,
		"bad entry":     `{"plugins":{"tailscale":{"id":"other","version":"1.0.0","sha256":"x"}}}`,
		"trailing data": `{} {}`,
	} {
		f := newFixture(t)
		path := filepath.Join(f.store.dir, stateName)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := f.store.Load(); !errors.Is(err, ErrStateCorrupt) {
			t.Fatalf("%s: got %v", name, err)
		}
		if err := f.install("1.0.0", programFile); !errors.Is(err, ErrStateCorrupt) {
			t.Fatalf("%s: install got %v", name, err)
		}
		if got, _ := os.ReadFile(path); string(got) != body {
			t.Fatalf("%s: corrupt file was rewritten", name)
		}
	}
}

func TestInterruptedInstallKeepsOldVersion(t *testing.T) {
	f := newFixture(t)
	if err := f.install("1.0.0", programFile); err != nil {
		t.Fatal(err)
	}
	orig := writeProgram
	t.Cleanup(func() { writeProgram = orig })
	writeProgram = func(path string, r io.Reader) error {
		data, _ := io.ReadAll(r)
		os.WriteFile(path, data[:len(data)/2], 0o700)
		return errors.New("disk full")
	}
	if err := f.install("1.1.0", []byte("version two")); err == nil {
		t.Fatal("interrupted install succeeded")
	}
	if p := f.state().Plugins["tailscale"]; p.Version != "1.0.0" || !p.Enabled {
		t.Fatalf("state changed: %+v", p)
	}
	if _, err := os.Stat(filepath.Join(f.store.dir, "tailscale", versionDir("1.1.0", fileHash([]byte("version two"))), programName)); !os.IsNotExist(err) {
		t.Fatalf("half-written program placed: %v", err)
	}
	if _, err := f.store.CheckStart("tailscale", f.startInput("1.0.0", programFile)); err != nil {
		t.Fatalf("old version no longer starts: %v", err)
	}
}

func TestRemove(t *testing.T) {
	f := newFixture(t)
	if err := f.install("1.0.0", programFile); err != nil {
		t.Fatal(err)
	}
	data := f.store.DataDir("tailscale")
	os.MkdirAll(data, 0o700)
	os.WriteFile(filepath.Join(data, "login"), []byte("x"), 0o600)

	if err := f.store.Remove("tailscale", RemoveDelete); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.state().Plugins["tailscale"]; ok {
		t.Fatal("delete left state")
	}
	if _, err := os.Stat(filepath.Join(data, "login")); !os.IsNotExist(err) {
		t.Fatalf("delete left data: %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.store.dir, "tailscale")); !os.IsNotExist(err) {
		t.Fatalf("delete left files: %v", err)
	}
	if err := f.store.Remove("tailscale", RemoveDelete); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("second remove: %v", err)
	}
	if err := f.install("1.0.0", programFile); err != nil {
		t.Fatal(err)
	}
	if err := f.store.Remove("tailscale", "wipe"); err == nil {
		t.Fatal("unknown mode accepted")
	}
}

func TestConcurrentInstallAndStart(t *testing.T) {
	f := newFixture(t)
	if err := f.install("1.0.0", programFile); err != nil {
		t.Fatal(err)
	}
	in := f.startInput("1.0.0", programFile)
	r := &recordRunner{}
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			errs <- f.store.Install(f.accepted("1.0.0", programFile))
		}()
		go func() {
			defer wg.Done()
			_, err := f.store.Start(context.Background(), r, "tailscale", in, nil, nil)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if p := f.state().Plugins["tailscale"]; !p.Enabled || len(r.Starts) != 10 {
		t.Fatalf("state %+v, starts %d", p, len(r.Starts))
	}
}

func TestReinstallSameVersionFailedSaveKeepsOld(t *testing.T) {
	f := newFixture(t)
	if err := f.install("1.0.0", programFile); err != nil {
		t.Fatal(err)
	}
	orig := writeState
	t.Cleanup(func() { writeState = orig })
	writeState = func(string, []byte) error { return errors.New("disk full") }
	if err := f.install("1.0.0", []byte("other bytes")); err == nil {
		t.Fatal("install with failed save succeeded")
	}
	writeState = orig
	if p := f.state().Plugins["tailscale"]; p.SHA256 != fileHash(programFile) {
		t.Fatalf("state changed: %+v", p)
	}
	r := &recordRunner{}
	if _, err := f.store.Start(context.Background(), r, "tailscale", f.startInput("1.0.0", programFile), nil, nil); err != nil {
		t.Fatalf("old program no longer starts: %v", err)
	}
}

func TestStartRefusesSymlinkedFolder(t *testing.T) {
	f := newFixture(t)
	if err := f.install("1.0.0", programFile); err != nil {
		t.Fatal(err)
	}
	verDir := filepath.Dir(f.programPath())
	other := t.TempDir()
	os.WriteFile(filepath.Join(other, programName), programFile, 0o700)
	if err := os.RemoveAll(verDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, verDir); err != nil {
		t.Skipf("symlinks not available: %v", err)
	}
	if _, err := f.store.CheckStart("tailscale", f.startInput("1.0.0", programFile)); !errors.Is(err, ErrTampered) {
		t.Fatalf("got %v, want ErrTampered", err)
	}
}

func TestExecRunnerRefusesRelativeProgram(t *testing.T) {
	if _, err := (ExecRunner{}).Start(context.Background(), Spec{Program: "program"}); err == nil {
		t.Fatal("relative program accepted")
	}
}

func TestRemoveCallsStop(t *testing.T) {
	f := newFixture(t)
	if err := f.install("1.0.0", programFile); err != nil {
		t.Fatal(err)
	}
	stopped := false
	if err := f.store.Remove("tailscale", RemoveDelete, func() error { stopped = true; return nil }); err != nil {
		t.Fatal(err)
	}
	if !stopped {
		t.Fatal("stop not called")
	}
	entries, _ := os.ReadDir(f.store.dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".trash-") {
			t.Fatalf("trash left: %s", e.Name())
		}
	}
}

// Without plugins nothing is written: the folder waits for the first install.
func TestNoFolderUntilInstall(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "plugins")
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	st, err := s.Load()
	if err != nil || len(st.Plugins) != 0 {
		t.Fatalf("load: %v %v", st, err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("folder made before an install: %v", err)
	}
}

// An old per-plugin "allow without the file lock" turns into the install-wide choice.
func TestStoreFoldsOldPerPluginFileLock(t *testing.T) {
	dir := t.TempDir()
	sum := strings.Repeat("a", 64)
	old := `{"list_counter":1,"block_counter":1,"plugins":{"x":{"id":"x","version":"1.0.0","sha256":"` + sum + `","dir":"1.0.0-` + sum[:16] + `","enabled":true,"allow_no_file_lock":true}}}`
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	st, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !st.AllowNoFileLock || !st.NoFileLockAsked || st.Plugins["x"].AllowNoFileLock {
		t.Fatalf("not folded: %+v %+v", st, st.Plugins["x"])
	}
}
