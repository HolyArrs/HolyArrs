package archive

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/importer/unrar"
)

// testdata/*.7z: one 390,000-byte file (noise, then text), packed by
// 7-Zip ("7zz a -t7z -mx=5"): whole, in two -v100k parts, and with a
// password over the names too (-mhe=on).
const (
	movie    = "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS.mkv"
	movieSum = "a287f9ac9d42fc7513f9ec16d9ab9c33ede55c29c93e74a9c953e5f7e415674f"
)

func sumOf(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func TestOf(t *testing.T) {
	for name, want := range map[string]struct {
		k     Kind
		first bool
	}{
		"a.7z": {SevenZip, true}, "a.7z.001": {SevenZip, true}, "a.7z.002": {SevenZip, false}, "A.ZIP": {Zip, true},
		"a.tar": {Tar, true}, "a.tar.gz": {TarGzip, true}, "a.tgz": {TarGzip, true}, "a.tar.bz2": {TarBzip2, true},
		"a.tar.xz": {TarXz, true}, "a.mkv.gz": {Gzip, true}, "a.xz": {Xz, true}, "a.rar": {"", false}, "a.mkv": {"", false},
	} {
		if k, first := Of(name); k != want.k || first != want.first {
			t.Errorf("%s: %q %v, want %q %v", name, k, first, want.k, want.first)
		}
	}
}

func TestSevenZipWholeAndInParts(t *testing.T) {
	for _, name := range []string{"one.7z", "parts.7z.001"} {
		dest := t.TempDir()
		if err := Extract(context.Background(), filepath.Join("testdata", name), SevenZip, dest, 1<<20); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := sumOf(t, filepath.Join(dest, movie)); got != movieSum {
			t.Fatalf("%s: wrong content", name)
		}
	}
}

func TestSevenZipWithAPasswordIsRefused(t *testing.T) {
	if err := Extract(context.Background(), filepath.Join("testdata", "pw.7z"), SevenZip, t.TempDir(), 1<<20); !errors.Is(err, unrar.ErrEncrypted) {
		t.Fatalf("got %v, want ErrEncrypted", err)
	}
}

func TestSevenZipPastTheCap(t *testing.T) {
	if err := Extract(context.Background(), filepath.Join("testdata", "one.7z"), SevenZip, t.TempDir(), 1000); !errors.Is(err, unrar.ErrTooBig) {
		t.Fatalf("got %v, want ErrTooBig", err)
	}
}

func writeZip(t *testing.T, files map[string][]byte, link string, encrypted bool) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "rel.zip")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, b := range files {
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		if encrypted {
			h.Flags |= 0x1
		}
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(b)
	}
	if link != "" {
		h := &zip.FileHeader{Name: link}
		h.SetMode(os.ModeSymlink | 0o777)
		w, _ := zw.CreateHeader(h)
		_, _ = w.Write([]byte("/etc/passwd"))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	return p
}

func TestZip(t *testing.T) {
	dest := t.TempDir()
	if err := Extract(context.Background(), writeZip(t, map[string][]byte{"Rel/" + movie: []byte("video"), "Rel/info.nfo": []byte("n")}, "", false), Zip, dest, 1<<20); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(dest, "Rel", movie)); err != nil || string(b) != "video" {
		t.Fatalf("%q %v", b, err)
	}
	for _, c := range []struct {
		name string
		path string
		err  error
		link string
		enc  bool
	}{
		{"climbs out", "../../evil.mkv", unrar.ErrUnsafePath, "", false},
		{"absolute", "/evil.mkv", unrar.ErrUnsafePath, "", false},
		{"link", "ok.mkv", unrar.ErrUnrecognised, "link.mkv", false},
		{"password", "ok.mkv", unrar.ErrEncrypted, "", true},
	} {
		dest := t.TempDir()
		err := Extract(context.Background(), writeZip(t, map[string][]byte{c.path: []byte("x")}, c.link, c.enc), Zip, dest, 1<<20)
		if !errors.Is(err, c.err) {
			t.Errorf("%s: %v, want %v", c.name, err, c.err)
		}
		if _, err := os.Stat(filepath.Join(filepath.Dir(dest), "evil.mkv")); err == nil {
			t.Errorf("%s: a file landed outside", c.name)
		}
	}
}

func TestTarGzipAndALinkInside(t *testing.T) {
	build := func(link bool) string {
		p := filepath.Join(t.TempDir(), "rel.tar.gz")
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gz)
		_ = tw.WriteHeader(&tar.Header{Name: "Rel/", Typeflag: tar.TypeDir, Mode: 0o755})
		_ = tw.WriteHeader(&tar.Header{Name: "Rel/" + movie, Typeflag: tar.TypeReg, Mode: 0o644, Size: 5})
		_, _ = tw.Write([]byte("video"))
		if link {
			_ = tw.WriteHeader(&tar.Header{Name: "Rel/sneaky", Typeflag: tar.TypeSymlink, Linkname: "/etc"})
		}
		_ = tw.Close()
		_ = gz.Close()
		if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	dest := t.TempDir()
	if err := Extract(context.Background(), build(false), TarGzip, dest, 1<<20); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "Rel", movie)); string(b) != "video" {
		t.Fatalf("content %q", b)
	}
	if err := Extract(context.Background(), build(true), TarGzip, t.TempDir(), 1<<20); !errors.Is(err, unrar.ErrUnrecognised) {
		t.Fatalf("link: %v", err)
	}
}

func TestSingleGzipAndDamage(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, _ = gz.Write([]byte("video"))
	_ = gz.Close()
	p := filepath.Join(dir, movie+".gz")
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	if err := Extract(context.Background(), p, Gzip, dest, 1<<20); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, movie)); string(b) != "video" {
		t.Fatalf("content %q", b)
	}
	bad := filepath.Join(dir, "bad.7z")
	_ = os.WriteFile(bad, []byte("7z\xbc\xaf\x27\x1cnot really"), 0o644)
	if err := Extract(context.Background(), bad, SevenZip, t.TempDir(), 1<<20); !errors.Is(err, unrar.ErrCorrupt) {
		t.Fatalf("damaged 7z: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Extract(ctx, filepath.Join("testdata", "one.7z"), SevenZip, t.TempDir(), 1<<20); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled: %v", err)
	}
}

// A 7-Zip archive may not ask its decoder for more than maxWindow: LZMA2,
// LZMA and PPMd each refuse a bigger ask before anything is allocated.
func TestSevenZipDecoderAsksAreCapped(t *testing.T) {
	src := func() []io.ReadCloser { return []io.ReadCloser{io.NopCloser(bytes.NewReader(nil))} }
	if _, err := lzma2Reader([]byte{40}, 0, src()); !errors.Is(err, unrar.ErrUnrecognised) {
		t.Errorf("LZMA2 3 GB dictionary: %v", err)
	}
	big := []byte{0x5d, 0, 0, 0, 0x40} // 1 GB dictionary
	if _, err := lzmaReader(big, 100, src()); !errors.Is(err, unrar.ErrUnrecognised) {
		t.Errorf("LZMA 1 GB dictionary: %v", err)
	}
	if _, err := ppmdReader([]byte{6, 0, 0, 0, 0x40}, 100, src()); !errors.Is(err, unrar.ErrUnrecognised) {
		t.Errorf("PPMd 1 GB model: %v", err)
	}
	// A usual size passes the check (the stream itself is empty here).
	if _, err := lzma2Reader([]byte{24}, 0, src()); errors.Is(err, unrar.ErrUnrecognised) {
		t.Errorf("LZMA2 16 MB dictionary refused: %v", err)
	}
}

// The PPMd decoder counts in int, so on a 32-bit build a file bigger than
// int can hold is refused instead of wrapping and ending early as damage.
func TestSevenZipPPMdSizePastIntIsRefused(t *testing.T) {
	stream := []io.ReadCloser{io.NopCloser(bytes.NewReader(make([]byte, 64)))}
	if _, err := ppmdReader([]byte{6, 0, 0, 0, 0x01}, uint64(math.MaxInt)+1, stream); err == nil {
		t.Fatal("a PPMd size past int was handed to the decoder")
	}
}
