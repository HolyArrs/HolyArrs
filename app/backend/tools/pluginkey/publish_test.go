package main

import (
	"crypto/ed25519"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/plugins"
)

func TestStageSignBlock(t *testing.T) {
	dir := t.TempDir()
	pass := []byte("a long test passphrase")
	pub, err := keygen(filepath.Join(dir, "key"), pass)
	if err != nil {
		t.Fatal(err)
	}
	old := trustKeys
	trustKeys = func() []ed25519.PublicKey { return []ed25519.PublicKey{pub} }
	defer func() { trustKeys = old }()
	keyPath := filepath.Join(dir, "key", keyFile)

	builds := filepath.Join(dir, "builds")
	for _, p := range []string{"linux-amd64", "linux-arm64"} {
		os.MkdirAll(filepath.Join(builds, p), 0o755)
		os.WriteFile(filepath.Join(builds, p, "plugin"), []byte("binary "+p), 0o644)
	}
	meta := filepath.Join(dir, "plugin.json")
	os.WriteFile(meta, []byte(`{"name":"Demo","author":"HolyArrs","access":{"hosts":["example.com"],"folders":["demo"],"actions":[]},"min_app_version":"1.1.5"}`), 0o644)

	live := filepath.Join(dir, "live")
	os.MkdirAll(live, 0o755)
	publish := func(stage string) {
		t.Helper()
		for _, k := range []string{"plugins", "blocklist"} {
			out := filepath.Join(live, k+".json")
			os.Remove(out)
			if err := sign(keyPath, pass, k, filepath.Join(stage, k+".payload.json"), out, 0); err != nil {
				t.Fatalf("sign %s: %v", k, err)
			}
		}
	}

	s1 := filepath.Join(dir, "s1")
	if err := runStage([]string{"-id", "demo", "-version", "1.0.0", "-meta", meta, "-builds", builds, "-current", live, "-out", s1}); err != nil {
		t.Fatal(err)
	}
	publish(s1)
	c, err := readCurrent(live)
	if err != nil {
		t.Fatal(err)
	}
	if c.list.Counter != 1 || len(c.list.Entries) != 2 || *c.list.MinBlocklistCounter != c.block.Counter {
		t.Fatalf("first list: %+v block %+v", c.list, c.block)
	}
	man, _ := os.ReadFile(filepath.Join(s1, "plugins", "demo", "1.0.0", "linux-arm64", "manifest.json"))
	if m, err := plugins.ParseManifest(man); err != nil || m.SHA256 != sum([]byte("binary linux-arm64")) {
		t.Fatalf("manifest: %v %+v", err, m)
	}

	if err := runStage([]string{"-id", "demo", "-version", "1.0.0", "-meta", meta, "-builds", builds, "-current", live, "-out", filepath.Join(dir, "dup")}); err == nil || !strings.Contains(err.Error(), "already on the list") {
		t.Fatalf("same version twice: %v", err)
	}

	s2 := filepath.Join(dir, "s2")
	if err := runBlock([]string{"-id", "demo", "-version", "1.0.0", "-current", live, "-out", s2}); err != nil {
		t.Fatal(err)
	}
	env, _ := os.ReadFile(filepath.Join(s2, "counters.env"))
	if !strings.Contains(string(env), "LIST_MIN=1") {
		t.Fatalf("counters.env: %s", env)
	}
	publish(s2)
	c2, _ := readCurrent(live)
	if c2.list.Counter != 2 || len(c2.list.Entries) != 0 || c2.block.Counter <= c.block.Counter || len(c2.block.Entries) != 1 {
		t.Fatalf("after block: list %+v block %+v", c2.list, c2.block)
	}
	raw, _ := json.Marshal(c2.block.Entries[0])
	if !strings.Contains(string(raw), `"demo"`) {
		t.Fatalf("block entry: %s", raw)
	}
}
