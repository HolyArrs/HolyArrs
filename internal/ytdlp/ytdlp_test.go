package ytdlp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// The test program doubles as a fake yt-dlp: with FAKE_YTDLP_VERSION set
// it prints that version and stops.
func TestMain(m *testing.M) {
	if v := os.Getenv("FAKE_YTDLP_VERSION"); v != "" {
		fmt.Println(v)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func release(t *testing.T, bin []byte, sum string) (*httptest.Server, *int) {
	t.Helper()
	downloads := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/SHA2-256SUMS":
			fmt.Fprintf(w, "%s  %s\n0000000000000000000000000000000000000000000000000000000000000000  other\n", sum, Asset())
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
	srv, downloads := release(t, bin, hex.EncodeToString(h[:]))
	dir := t.TempDir()
	tool := &Tool{DataDir: dir, ReleaseBase: srv.URL + "/"}
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
	srv, _ := release(t, []byte("tampered"), "1111111111111111111111111111111111111111111111111111111111111111")
	dir := t.TempDir()
	tool := &Tool{DataDir: dir, ReleaseBase: srv.URL + "/"}
	if _, err := tool.Update(context.Background()); err == nil {
		t.Fatal("bad checksum accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, exe())); err == nil {
		t.Fatal("tampered copy installed")
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
	if tool.Find() != filepath.Join(dir, exe()) {
		t.Fatal("bundled copy wins over the data copy")
	}
	t.Setenv(Env, filepath.Join(dir, "missing"))
	if tool.Find() != "" {
		t.Fatal("a named but missing program is not replaced")
	}
}
