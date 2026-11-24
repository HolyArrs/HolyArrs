package backups

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// rawZip writes one entry whose header declares `declared` bytes while the
// deflated data really holds `real`.
func rawZip(t *testing.T, name string, real []byte, declared uint64) []byte {
	t.Helper()
	var comp bytes.Buffer
	fw, _ := flate.NewWriter(&comp, flate.BestCompression)
	_, _ = fw.Write(real)
	_ = fw.Close()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	meta, _ := w.Create(MetaFile)
	_, _ = meta.Write([]byte(`{"app":"holyarrs"}`))
	h := &zip.FileHeader{Name: name, Method: zip.Deflate, CompressedSize64: uint64(comp.Len()), UncompressedSize64: declared}
	f, err := w.CreateRaw(h)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write(comp.Bytes())
	_ = w.Close()
	return buf.Bytes()
}

func noStaged(t *testing.T, m *Manager, what string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(m.Dir, StagedDir)); !os.IsNotExist(err) {
		t.Fatalf("%s: left a staged folder", what)
	}
	left, _ := filepath.Glob(filepath.Join(m.Dir, ".restore-*"))
	if len(left) > 0 {
		t.Fatalf("%s: left temp folders %v", what, left)
	}
}

// A zip that declares a huge unpacked size is refused before anything is
// written, and a declared size near uint64's top does not wrap around.
func TestAdvDeclaredZipBombRefusedUpFront(t *testing.T) {
	m, _, _, _ := setup(t)
	m.FreeSpace = func(string) int64 { return 64 << 30 }
	for _, declared := range []uint64{300 << 30, ^uint64(0)} {
		z := rawZip(t, database.MainFile, []byte("x"), declared)
		err := m.Stage(context.Background(), bytes.NewReader(z))
		if err == nil || !(strings.Contains(err.Error(), "too large") || strings.Contains(err.Error(), "Not enough free space")) {
			t.Fatalf("declared %d: got %v", declared, err)
		}
		noStaged(t, m, "declared bomb")
	}
}

// A zip that lies small about its size still stops at the cap.
func TestAdvLyingSizeStopsAtTheCap(t *testing.T) {
	m, _, _, _ := setup(t)
	old := maxUnpacked
	maxUnpacked = 4096
	defer func() { maxUnpacked = old }()
	z := rawZip(t, database.MainFile, bytes.Repeat([]byte{0}, 1<<20), 10)
	if err := m.Stage(context.Background(), bytes.NewReader(z)); err == nil {
		t.Fatal("a zip that unpacks past the cap was accepted")
	}
	noStaged(t, m, "lying size")
}

// Entry names that try to leave the restore folder are refused.
func TestAdvPathTricksRefused(t *testing.T) {
	m, _, _, _ := setup(t)
	for _, n := range []string{"../main.db", "./main.db", "x/../main.db", "/main.db", `..\main.db`, "MAIN.DB"} {
		z := zipBytes(t, map[string][]byte{MetaFile: []byte(`{"app":"holyarrs"}`), n: []byte("x")})
		if err := m.Stage(context.Background(), bytes.NewReader(z)); err == nil {
			t.Fatalf("%q accepted", n)
		}
		noStaged(t, m, n)
	}
}

// A broken new restore keeps the restore staged before it.
func TestAdvFailedRestoreKeepsTheEarlierOne(t *testing.T) {
	m, _, _, _ := setup(t)
	good, err := m.Create(context.Background(), KindManual)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(good.Path)
	if err := m.Stage(context.Background(), bytes.NewReader(b)); err != nil {
		t.Fatal(err)
	}
	if err := m.Stage(context.Background(), bytes.NewReader(b[:len(b)/2])); err == nil {
		t.Fatal("half a zip was accepted")
	}
	if _, err := os.Stat(filepath.Join(m.Dir, StagedDir, readyFile)); err != nil {
		t.Fatalf("the earlier staged restore was lost: %v", err)
	}
}

// An upload that stops sending fails after the stall time and leaves nothing.
func TestAdvStalledUploadFails(t *testing.T) {
	m, _, _, _ := setup(t)
	oldStall := uploadStall
	uploadStall = 200 * time.Millisecond
	defer func() { uploadStall = oldStall }()
	mux := http.NewServeMux()
	m.Routes(mux, nil)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	pr, pw := io.Pipe()
	go func() {
		_, _ = pw.Write([]byte("PK\x03\x04"))
		time.Sleep(2 * time.Second)
		pw.Close()
	}()
	start := time.Now()
	resp, err := http.Post(srv.URL+RestorePath, "application/zip", pr)
	if err == nil {
		resp.Body.Close()
		if resp.StatusCode == http.StatusAccepted {
			t.Fatal("a stalled upload was accepted")
		}
	}
	if d := time.Since(start); d > 1900*time.Millisecond {
		t.Fatalf("the stall was not cut off: %s", d)
	}
	time.Sleep(100 * time.Millisecond)
	noStaged(t, m, "stall")
}
