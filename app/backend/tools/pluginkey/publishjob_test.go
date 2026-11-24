package main

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/plugins"
)

// TestPublishJob runs the release-tools publishing script in dry-run mode
// with a throwaway key and checks its output with the engine's own code:
// empty first lists, two publishes, then a block.
func TestPublishJob(t *testing.T) {
	_, here, _, _ := runtime.Caller(0)
	tools := filepath.Join(filepath.Dir(here), "..", "..", "..", "..", "deploy", "release", "plugins")
	if _, err := os.Stat(filepath.Join(tools, "publish.py")); err != nil {
		t.Skip("deploy/release/plugins not in this checkout")
	}
	py := ""
	for _, name := range []string{"python3", "python"} {
		// Windows has a python3 shortcut that only opens the Store.
		if p, err := exec.LookPath(name); err == nil && exec.Command(p, "--version").Run() == nil {
			py = p
			break
		}
	}
	if py == "" {
		t.Skip("python not installed")
	}
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "pluginkey")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	// A throwaway plain key, the way CI holds the real one: no passphrase.
	keyDir := filepath.Join(tmp, "key")
	if err := os.MkdirAll(keyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	var secret bytes.Buffer
	pub, err := keygenPlain(&secret, filepath.Join(keyDir, pubFile))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(keyDir, keyFile), secret.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	seed := strings.TrimSpace(secret.String())

	// tree is the plugins repo checkout; live is what the site serves. After
	// each run the tree is "deployed" (copied to live) and snapshotted.
	tree := filepath.Join(tmp, "tree")
	live := filepath.Join(tmp, "live")
	step := 0
	job := func(args ...string) *exec.Cmd {
		return exec.Command(py, append([]string{filepath.Join(tools, "publish.py")}, append(args,
			"--pluginkey", bin, "--key", filepath.Join(keyDir, keyFile),
			"--main-pub", filepath.Join(keyDir, pubFile),
			"--live-url", live, "--out", tree)...)...)
	}
	runJob := func(args ...string) string {
		t.Helper()
		step++
		got, err := job(args...).CombinedOutput()
		if strings.Contains(string(got), seed) {
			t.Fatal("the private key was printed")
		}
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, got)
		}
		t.Logf("%v:\n%s", args, got)
		snap := filepath.Join(tmp, "snap", string(rune('0'+step)))
		copyTree(t, tree, snap)
		_ = os.RemoveAll(live)
		copyTree(t, tree, live)
		return snap
	}
	lists := func(dir string) (*plugins.List, *plugins.List) {
		t.Helper()
		l, err := plugins.VerifyList(pub, read(t, dir, "plugins.json"), plugins.KindPlugins, 0)
		if err != nil {
			t.Fatal(err)
		}
		b, err := plugins.VerifyList(pub, read(t, dir, "blocklist.json"), plugins.KindBlocklist, 0)
		if err != nil {
			t.Fatal(err)
		}
		return l, b
	}
	install := func(dir, verDir string) error {
		f, err := os.Open(filepath.Join(verDir, "plugin"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		acc, err := plugins.CheckInstall(plugins.InstallInput{PublicKey: pub, List: read(t, dir, "plugins.json"),
			Blocklist: read(t, dir, "blocklist.json"), Manifest: read(t, verDir, "manifest.json"), File: f, TempDir: tmp})
		if err == nil {
			acc.Close()
		}
		return err
	}

	// 1. First run: empty signed lists.
	init := runJob("init")
	l, b := lists(init)
	if l.Counter != 1 || b.Counter != 1 || len(l.Entries) != 0 || len(b.Entries) != 0 {
		t.Fatalf("init: list %+v, blocklist %+v", l, b)
	}

	// 2. Two publishes: the counter goes up each time.
	src := filepath.Join(tools, "test", "example-plugin")
	v1 := runJob("publish", "--id", "example", "--version", "1.0.0", "--src", src)
	v1Dir := filepath.Join(v1, "plugins", "example", "1.0.0")
	// The picture robot's logo copy in the tree: the manifest names it, not an inline icon.
	if err := os.MkdirAll(filepath.Join(tree, "images", "example"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"128", "256"} {
		if err := os.WriteFile(filepath.Join(tree, "images", "example", "icon-"+s+".webp"), []byte("RIFF....WEBP"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	v2 := runJob("publish", "--id", "example", "--version", "1.0.1", "--src", src)
	v2Dir := filepath.Join(v2, "plugins", "example", "1.0.1")
	if m, err := plugins.ParseManifest(read(t, v2Dir, "manifest.json")); err != nil || m.IconURL != "images/example/icon-256.webp" || m.Icon != "" {
		t.Fatalf("manifest with the robot's logo: %v %+v", err, m)
	}
	if m, err := plugins.ParseManifest(read(t, v1Dir, "manifest.json")); err != nil || m.IconURL != "" {
		t.Fatalf("manifest without a logo copy: %v %+v", err, m)
	}
	l1, _ := lists(v1)
	l2, _ := lists(v2)
	if l1.Counter != 2 || l2.Counter != 3 || len(l2.Entries) != 2 {
		t.Fatalf("counters %d then %d, entries %d", l1.Counter, l2.Counter, len(l2.Entries))
	}
	// A newer list is accepted after an older one, never the other way.
	if _, err := plugins.VerifyList(pub, read(t, v1, "plugins.json"), plugins.KindPlugins, l2.Counter); !errors.Is(err, plugins.ErrListOlder) {
		t.Fatalf("old list after new: %v", err)
	}
	if err := install(v2, v1Dir); err != nil {
		t.Fatalf("1.0.0 before the block: %v", err)
	}
	// Every platform has its own list at the same counter, and each build
	// installs only through its own platform's list.
	for _, plat := range plugins.Platforms {
		pl, err := plugins.VerifyList(pub, read(t, v2, plugins.ListName(plat)), plugins.KindPlugins, l1.Counter)
		if err != nil || pl.Counter != l2.Counter || len(pl.Entries) != 2 {
			t.Fatalf("%s list: %+v, %v", plat, pl, err)
		}
		dir := filepath.Join(v2, filepath.FromSlash(plugins.FileBase("example", "1.0.1", plat)))
		for _, other := range plugins.Platforms {
			f, err := os.Open(filepath.Join(dir, "plugin"))
			if err != nil {
				t.Fatal(err)
			}
			acc, err := plugins.CheckInstall(plugins.InstallInput{PublicKey: pub, List: read(t, v2, plugins.ListName(other)),
				Blocklist: read(t, v2, "blocklist.json"), Manifest: read(t, dir, "manifest.json"), File: f, TempDir: tmp})
			f.Close()
			if err == nil {
				acc.Close()
			}
			if (other == plat) != (err == nil) {
				t.Fatalf("%s build with the %s list: %v", plat, other, err)
			}
		}
	}

	// Publishing the same version again is refused.
	if out, err := job("publish", "--id", "example", "--version", "1.0.1", "--src", src).CombinedOutput(); err == nil || !strings.Contains(string(out), "already published") {
		t.Fatalf("republish was not refused: %v\n%s", err, out)
	}

	// A stale tree (live counters ahead of it) is refused and left untouched.
	_ = os.RemoveAll(tree)
	copyTree(t, v1, tree)
	if out, err := job("publish", "--id", "example", "--version", "1.0.2", "--src", src).CombinedOutput(); err == nil || !strings.Contains(string(out), "ahead of the tree") {
		t.Fatalf("stale tree was not refused: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(tree, "plugins", "example", "1.0.2")); !os.IsNotExist(err) {
		t.Fatal("a refused run wrote into the tree")
	}
	_ = os.RemoveAll(tree)
	copyTree(t, v2, tree)

	// 3. Block 1.0.0: it is refused, 1.0.1 still installs.
	blk := runJob("block", "--id", "example", "--version", "1.0.0", "--reason", "test block")
	l3, b3 := lists(blk)
	if b3.Counter != 2 || l3.Counter != 4 || *l3.MinBlocklistCounter != 2 {
		t.Fatalf("after block: list %d (min block %d), blocklist %d", l3.Counter, *l3.MinBlocklistCounter, b3.Counter)
	}
	if err := install(blk, v1Dir); !errors.Is(err, plugins.ErrBlocked) {
		t.Fatalf("blocked 1.0.0: got %v", err)
	}
	if err := install(blk, v2Dir); err != nil {
		t.Fatalf("1.0.1 after the block: %v", err)
	}
	for _, plat := range plugins.Platforms {
		pl, err := plugins.VerifyList(pub, read(t, blk, plugins.ListName(plat)), plugins.KindPlugins, 0)
		if err != nil || pl.Counter != l3.Counter || *pl.MinBlocklistCounter != b3.Counter {
			t.Fatalf("%s list after block: %+v, %v", plat, pl, err)
		}
	}
	// The new plugin list refuses the old blocklist that did not block 1.0.0.
	f, _ := os.Open(filepath.Join(v1Dir, "plugin"))
	defer f.Close()
	_, err = plugins.CheckInstall(plugins.InstallInput{PublicKey: pub, List: read(t, blk, "plugins.json"),
		Blocklist: read(t, v2, "blocklist.json"), Manifest: read(t, v1Dir, "manifest.json"), File: f, TempDir: tmp})
	if !errors.Is(err, plugins.ErrStaleBlock) {
		t.Fatalf("old blocklist with new list: %v", err)
	}
	// Lists signed by another key are refused.
	other, _, _ := ed25519.GenerateKey(nil)
	if _, err := plugins.VerifyList(other, read(t, blk, "plugins.json"), plugins.KindPlugins, 0); err == nil {
		t.Fatal("list verified with an unrelated key")
	}
}

func copyTree(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.Walk(from, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		dst := filepath.Join(to, rel)
		if info.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, dir, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}
