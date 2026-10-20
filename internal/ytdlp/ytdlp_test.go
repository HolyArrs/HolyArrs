package ytdlp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/openpgp"
	"golang.org/x/crypto/openpgp/packet"
)

// The test program doubles as a fake yt-dlp: with FAKE_YTDLP_VERSION set
// it prints that version and stops.
func TestMain(m *testing.M) {
	EnvAllow = append(EnvAllow, "FAKE_YTDLP_VERSION")
	switch os.Getenv("FAKE_YTDLP_ENV") {
	case "1":
		fmt.Print(strings.Join(os.Environ(), "\n"))
		os.Exit(0)
	case "big":
		os.Stdout.Write(bytes.Repeat([]byte("x"), MaxOutput+10))
		os.Exit(0)
	}
	if v := os.Getenv("FAKE_YTDLP_VERSION"); v != "" {
		fmt.Println(v)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// signer is the fake release key; stranger is some other key.
var signer, stranger *openpgp.Entity

func keys(t *testing.T) {
	t.Helper()
	if signer != nil {
		return
	}
	var err error
	cfg := &packet.Config{RSABits: 2048}
	if signer, err = openpgp.NewEntity("release", "", "release@example.invalid", cfg); err != nil {
		t.Fatal(err)
	}
	if stranger, err = openpgp.NewEntity("stranger", "", "stranger@example.invalid", cfg); err != nil {
		t.Fatal(err)
	}
}

func sign(t *testing.T, by *openpgp.Entity, msg []byte) []byte {
	t.Helper()
	var sig bytes.Buffer
	if err := openpgp.DetachSign(&sig, by, bytes.NewReader(msg), nil); err != nil {
		t.Fatal(err)
	}
	return sig.Bytes()
}

// release serves a fake release whose sums are signed by `by` (nil: no
// signature file).
func release(t *testing.T, bin []byte, sum string, by *openpgp.Entity) (*httptest.Server, *int) {
	t.Helper()
	downloads := 0
	sums := []byte(fmt.Sprintf("%s  %s\n0000000000000000000000000000000000000000000000000000000000000000  other\n", sum, Asset()))
	var sig []byte
	if by != nil {
		sig = sign(t, by, sums)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/SHA2-256SUMS":
			w.Write(sums)
		case "/SHA2-256SUMS.sig":
			if sig == nil {
				w.WriteHeader(404)
				return
			}
			w.Write(sig)
		case "/" + Asset():
			downloads++
			w.Write(bin)
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &downloads
}

func self(t *testing.T) []byte {
	b, err := os.ReadFile(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestUpdateVerifiesSwapsAndRollsBack(t *testing.T) {
	if Asset() == "" {
		t.Skip("no release for this machine")
	}
	t.Setenv("FAKE_YTDLP_VERSION", "2099.01.01")
	bin := self(t)
	h := sha256.Sum256(bin)
	keys(t)
	srv, downloads := release(t, bin, hex.EncodeToString(h[:]), signer)
	dir := t.TempDir()
	tool := &Tool{DataDir: dir, ReleaseBase: srv.URL + "/", Keyring: openpgp.EntityList{signer}}
	v, err := tool.Update(context.Background())
	if err != nil || v != "2099.01.01" {
		t.Fatalf("update %q %v", v, err)
	}
	if res, at := tool.LastResult(); res != "up to date (2099.01.01)" || at.IsZero() {
		t.Fatal(res)
	}
	dest := filepath.Join(dir, exe())
	if tool.Find() != dest && os.Getenv(Env) == "" {
		t.Fatalf("find %s", tool.Find())
	}
	// Same release again: nothing downloaded.
	if _, err := tool.Update(context.Background()); err != nil || *downloads != 1 {
		t.Fatalf("second %v %d", err, *downloads)
	}
	// A new copy that fails the canary: the previous one comes back.
	os.WriteFile(dest, []byte("old copy"), 0o755)
	tool.Canary = func(context.Context, string) error { return errors.New("no formats") }
	if _, err := tool.Update(context.Background()); err == nil {
		t.Fatal("canary failure accepted")
	}
	if b, _ := os.ReadFile(dest); string(b) != "old copy" {
		t.Fatal("previous copy not restored")
	}
	if res, _ := tool.LastResult(); res == "" || res[:13] != "update failed" {
		t.Fatal(res)
	}
}

func TestUpdateRefusesBadChecksum(t *testing.T) {
	if Asset() == "" {
		t.Skip("no release for this machine")
	}
	keys(t)
	// (c) correctly signed sums, but the binary does not match them.
	srv, _ := release(t, []byte("tampered"), "1111111111111111111111111111111111111111111111111111111111111111", signer)
	dir := t.TempDir()
	dest := filepath.Join(dir, exe())
	os.WriteFile(dest, []byte("current copy"), 0o755)
	tool := &Tool{DataDir: dir, ReleaseBase: srv.URL + "/", Keyring: openpgp.EntityList{signer}}
	if _, err := tool.Update(context.Background()); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("bad checksum accepted: %v", err)
	}
	if b, _ := os.ReadFile(dest); string(b) != "current copy" {
		t.Fatal("current copy not kept")
	}
}

func TestUpdateRefusesBadOrMissingSignature(t *testing.T) {
	if Asset() == "" {
		t.Skip("no release for this machine")
	}
	keys(t)
	bin := []byte("would-be yt-dlp")
	h := sha256.Sum256(bin)
	sum := hex.EncodeToString(h[:])
	for name, by := range map[string]*openpgp.Entity{"signed by another key": stranger, "no signature": nil} {
		srv, downloads := release(t, bin, sum, by)
		dir := t.TempDir()
		dest := filepath.Join(dir, exe())
		os.WriteFile(dest, []byte("current copy"), 0o755)
		tool := &Tool{DataDir: dir, ReleaseBase: srv.URL + "/", Keyring: openpgp.EntityList{signer}}
		if _, err := tool.Update(context.Background()); err == nil {
			t.Fatalf("%s: accepted", name)
		}
		if *downloads != 0 {
			t.Fatalf("%s: binary downloaded before the signature passed", name)
		}
		if b, _ := os.ReadFile(dest); string(b) != "current copy" {
			t.Fatalf("%s: current copy not kept", name)
		}
	}
	// Sums changed after signing.
	sums := []byte(sum + "  " + Asset() + "\n")
	sig := sign(t, signer, []byte("something else\n"))
	if err := verifySums(openpgp.EntityList{signer}, sums, sig); err == nil {
		t.Fatal("altered sums accepted")
	}
}

// The pinned key is yt-dlp's real release key: it verifies the real
// 2026.08.19 sums and refuses them once one byte changes.
func TestPinnedKeyVerifiesRealRelease(t *testing.T) {
	sums, err := os.ReadFile("testdata/SHA2-256SUMS-2026.08.19")
	if err != nil {
		t.Fatal(err)
	}
	sig, err := os.ReadFile("testdata/SHA2-256SUMS-2026.08.19.sig")
	if err != nil {
		t.Fatal(err)
	}
	kr, err := pinnedKeyring()
	if err != nil {
		t.Fatal(err)
	}
	if err := verifySums(kr, sums, sig); err != nil {
		t.Fatalf("real release refused: %v", err)
	}
	sums[0] ^= 1
	if err := verifySums(kr, sums, sig); err == nil {
		t.Fatal("altered real sums accepted")
	}
}

// Run hands yt-dlp only PATH, HOME, the temp folder and proxies, and stops
// reading at MaxOutput.
func TestRunEnvAndOutputCap(t *testing.T) {
	t.Setenv("HOLYARRS_TMDB_READ_TOKEN", "secret-value")
	t.Setenv("HTTPS_PROXY", "http://proxy.invalid:3128")
	t.Setenv("FAKE_YTDLP_ENV", "1")
	EnvAllow = append(EnvAllow, "FAKE_YTDLP_ENV")
	t.Cleanup(func() { EnvAllow = EnvAllow[:len(EnvAllow)-1] })
	out, err := Run(context.Background(), os.Args[0], 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	env := string(out)
	if strings.Contains(env, "secret-value") || strings.Contains(env, "HOLYARRS_") {
		t.Fatalf("secret passed to yt-dlp: %s", env)
	}
	if !strings.Contains(env, "PATH=") || !strings.Contains(env, "HTTPS_PROXY=http://proxy.invalid:3128") {
		t.Fatalf("missing PATH or proxy: %s", env)
	}
	t.Setenv("FAKE_YTDLP_ENV", "big")
	if _, err := Run(context.Background(), os.Args[0], 30*time.Second); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("oversize answer: %v", err)
	}
}

func TestFindOrder(t *testing.T) {
	dir := t.TempDir()
	data := filepath.Join(dir, "data")
	os.MkdirAll(data, 0o755)
	os.WriteFile(filepath.Join(data, exe()), []byte("x"), 0o755)
	tool := &Tool{DataDir: data, Self: filepath.Join(dir, "holyarrs")}
	t.Setenv(Env, "")
	t.Setenv("PATH", "")
	if tool.Find() != filepath.Join(data, exe()) {
		t.Fatal("data copy")
	}
	os.WriteFile(filepath.Join(dir, exe()), []byte("x"), 0o755)
	if tool.Find() != filepath.Join(data, exe()) {
		t.Fatal("the updated copy wins over the bundled one")
	}
	os.Remove(filepath.Join(data, exe()))
	if tool.Find() != filepath.Join(dir, exe()) {
		t.Fatal("bundled copy")
	}
	t.Setenv(Env, filepath.Join(dir, "missing"))
	if tool.Find() != "" {
		t.Fatal("a named but missing program is not replaced")
	}
}
