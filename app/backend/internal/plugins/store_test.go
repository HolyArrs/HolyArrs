package plugins

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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
		return sign(t, priv, l)
	}
	return f
}

// install runs CheckInstall and Install for one version of the program.
func (f *fixture) install(version string, file []byte) error {
	f.t.Helper()
	man := manifestJSON(func(m map[string]any) { m["version"] = version; m["sha256"] = fileHash(file) })
	acc, err := CheckInstall(InstallInput{
		PublicKey: f.pub,
		List:      f.sign(KindPlugins, 1, Entry{ID: "tailscale", Version: version, SHA256: fileHash(file)}),
		Blocklist: f.sign(KindBlocklist, 1),
		Manifest:  man,
		File:      strings.NewReader(string(file)),
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return f.store.Install(acc)
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

func TestInstallAndStart(t *testing.T) {
	f := newFixture(t)
	if err := f.install("1.0.0", programFile); err != nil {
		t.Fatal(err)
	}
	p := f.state().Plugins["tailscale"]
	if p == nil || !p.Enabled || p.Version != "1.0.0" || p.SHA256 != fileHash(programFile) {
		t.Fatalf("state: %+v", p)
	}
	r := &FakeRunner{}
	proc, err := f.store.Start(context.Background(), r, "tailscale", f.startInput("1.0.0", programFile), []string{"up"}, []string{"A=1"})
	if err != nil {
		t.Fatal(err)
	}
	proc.Stop()
	if len(r.Starts) != 1 || r.Starts[0].DataDir != f.store.DataDir("tailscale") ||
		r.Starts[0].Program != f.store.ProgramPath("tailscale", "1.0.0") {
		t.Fatalf("starts: %+v", r.Starts)
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
	if err := f.install("1.1.0", newFile); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(f.store.ProgramPath("tailscale", "1.0.0"))); !os.IsNotExist(err) {
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
	if err := os.WriteFile(f.store.ProgramPath("tailscale", "1.0.0"), []byte("evil"), 0o700); err != nil {
		t.Fatal(err)
	}
	r := &FakeRunner{}
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
	os.Remove(f.store.ProgramPath("tailscale", "1.0.0"))
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
	writeProgram = func(path string, data []byte) error {
		os.WriteFile(path, data[:len(data)/2], 0o700)
		return errors.New("disk full")
	}
	if err := f.install("1.1.0", []byte("version two")); err == nil {
		t.Fatal("interrupted install succeeded")
	}
	if p := f.state().Plugins["tailscale"]; p.Version != "1.0.0" || !p.Enabled {
		t.Fatalf("state changed: %+v", p)
	}
	if _, err := os.Stat(f.store.ProgramPath("tailscale", "1.1.0")); !os.IsNotExist(err) {
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

	if err := f.store.Remove("tailscale", RemoveKeep); err != nil {
		t.Fatal(err)
	}
	if p := f.state().Plugins["tailscale"]; p == nil || p.Enabled {
		t.Fatalf("keep: %+v", p)
	}
	if _, err := os.Stat(filepath.Join(data, "login")); err != nil {
		t.Fatalf("keep lost data: %v", err)
	}
	if _, err := f.store.CheckStart("tailscale", f.startInput("1.0.0", programFile)); !errors.Is(err, ErrDisabled) {
		t.Fatalf("kept plugin started: %v", err)
	}

	if err := f.store.Remove("tailscale", RemoveDelete); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.state().Plugins["tailscale"]; ok {
		t.Fatal("delete left state")
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
