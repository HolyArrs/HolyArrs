package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/updates"
)

// A plain key goes only to the writer; the public file matches it, and a
// list and a program signed with it pass the app's own checks.
func TestPlainKeySignsListAndProgram(t *testing.T) {
	dir := t.TempDir()
	pubPath := filepath.Join(dir, "keys", "main.pub")
	var secret bytes.Buffer
	k, err := keygenPlain(&secret, pubPath)
	if err != nil {
		t.Fatal(err)
	}
	pubText, _ := os.ReadFile(pubPath)
	if strings.Contains(string(pubText), strings.TrimSpace(secret.String())) {
		t.Fatal("the public file holds the private key")
	}
	if _, err := keygenPlain(&bytes.Buffer{}, pubPath); err == nil {
		t.Fatal("keygen overwrote an existing public key")
	}
	keyPath := filepath.Join(dir, "update.key")
	if err := os.WriteFile(keyPath, secret.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	dist := filepath.Join(dir, "dist")
	_ = os.MkdirAll(dist, 0o755)
	prog := filepath.Join(dist, "holyarrs_1.2.1_linux_amd64")
	_ = os.WriteFile(prog, []byte("\x7fELF program"), 0o755)
	if err := signFile(keyPath, "", prog, prog+updates.SigSuffix, true); err == nil {
		t.Fatal("a program was signed as a list")
	}
	if err := signFile(keyPath, "", prog, prog+updates.SigSuffix, false); err != nil {
		t.Fatal(err)
	}
	f := listFlags{version: "1.2.1", dist: dist, base: "https://github.com/HolyArrs/HolyArrs/releases/download/1.2.1", out: filepath.Join(dir, "manifest.json"), safe: true}
	if err := runList([]string{"-version", f.version, "-dist", f.dist, "-base", f.base, "-out", f.out, "-auto-safe", "-notes", "<b>Fixes</b>"}); err != nil {
		t.Fatal(err)
	}
	if err := signFile(keyPath, "", f.out, f.out+updates.SigSuffix, true); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(f.out)
	sig, _ := os.ReadFile(f.out + updates.SigSuffix)
	l, err := updates.Parse(data, sig, []updates.PublicKey{k}, time.Now(), 0)
	if err != nil {
		t.Fatal(err)
	}
	c := l.Channels["stable"]
	file, ok := c.Files["linux-amd64"]
	if !ok || file.Size != int64(len("\x7fELF program")) || !c.AutoUpdateSafe || c.NotesShort != "Fixes" ||
		c.ChangelogURL != "https://github.com/HolyArrs/HolyArrs/releases/tag/1.2.1" {
		t.Fatalf("list = %+v", c)
	}
	progSig, _ := os.ReadFile(prog + updates.SigSuffix)
	progData, _ := os.ReadFile(prog)
	if _, err := updates.Verify(progData, progSig, []updates.PublicKey{k}); err != nil {
		t.Fatal(err)
	}
}

// One-way releases are never marked safe for Automatic; a program without
// a signature stops the list.
func TestListRules(t *testing.T) {
	dist := t.TempDir()
	prog := filepath.Join(dist, "holyarrs_1.2.1_linux_arm64")
	_ = os.WriteFile(prog, []byte("x"), 0o755)
	f := listFlags{version: "1.2.1", dist: dist, base: "https://github.com/HolyArrs/HolyArrs/releases/download/1.2.1", safe: true, oneWay: true}
	if _, err := makeList(f, time.Now()); err == nil {
		t.Fatal("an unsigned program was listed")
	}
	_ = os.WriteFile(prog+updates.SigSuffix, []byte("sig"), 0o644)
	l, err := makeList(f, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if c := l.Channels["stable"]; c.AutoUpdateSafe || !c.OneWay {
		t.Fatalf("one-way release: %+v", c)
	}
	f.base = "https://example.com/files"
	if _, err := makeList(f, time.Now()); err == nil {
		t.Fatal("a host the app refuses was listed")
	}
}
