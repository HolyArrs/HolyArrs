package unrar

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const (
	rar5Name = "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS.mkv"
	rar5Sum  = "5adbec0ebd8cf98c55cdd2df2428fa562e63de8312dc7fdb46e01e8504cfc34b"
	rar5Size = 720000
)

func rar5First() string {
	return filepath.Join("testdata", "rar5-compressed", "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS.part01.rar")
}

// TestList_CompressedMultiVolumeRAR5 lists a compressed RAR 5 set.
func TestList_CompressedMultiVolumeRAR5(t *testing.T) {
	es, err := List(rar5First())
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 1 || es[0].Name != rar5Name || es[0].Size != rar5Size || !es[0].Compressed || es[0].Encrypted || es[0].Link {
		t.Fatalf("entries %+v", es)
	}
	if _, err := Extract(rar5First(), t.TempDir()); !errors.Is(err, ErrCompressed) {
		t.Fatalf("the stored-set reader took a compressed set: %v", err)
	}
}

// TestExtractCompressed_MultiVolumeRAR5 unpacks a compressed RAR 5 set over four volumes.
func TestExtractCompressed_MultiVolumeRAR5(t *testing.T) {
	dest := t.TempDir()
	if err := ExtractCompressed(context.Background(), rar5First(), dest, rar5Size); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dest, rar5Name))
	if err != nil {
		t.Fatal(err)
	}
	if sum := sha256.Sum256(b); hex.EncodeToString(sum[:]) != rar5Sum {
		t.Fatalf("unpacked %d bytes with the wrong content", len(b))
	}
}

// TestExtractCompressed_PastTheLimit stops once the limit is passed.
func TestExtractCompressed_PastTheLimit(t *testing.T) {
	if err := ExtractCompressed(context.Background(), rar5First(), t.TempDir(), rar5Size-1); !errors.Is(err, ErrTooBig) {
		t.Fatalf("got %v, want ErrTooBig", err)
	}
}

// TestExtractCompressed_ExistingFile never overwrites an existing file.
func TestExtractCompressed_ExistingFile(t *testing.T) {
	dest := t.TempDir()
	if err := os.WriteFile(filepath.Join(dest, rar5Name), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ExtractCompressed(context.Background(), rar5First(), dest, rar5Size); err == nil {
		t.Fatal("an existing file was overwritten")
	}
	if b, _ := os.ReadFile(filepath.Join(dest, rar5Name)); string(b) != "mine" {
		t.Fatal("the existing file changed")
	}
}

// TestExtractCompressed_MissingVolume reports a missing volume as damage.
func TestExtractCompressed_MissingVolume(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"part01", "part02", "part04"} {
		b, err := os.ReadFile(filepath.Join("testdata", "rar5-compressed", "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS."+n+".rar"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "set."+n+".rar"), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := ExtractCompressed(context.Background(), filepath.Join(dir, "set.part01.rar"), t.TempDir(), rar5Size); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("got %v, want ErrCorrupt", err)
	}
}

// TestExtractCompressed_ChangedByte reports a changed byte as damage.
func TestExtractCompressed_ChangedByte(t *testing.T) {
	dir := t.TempDir()
	for i, n := range []string{"part01", "part02", "part03", "part04"} {
		b, err := os.ReadFile(filepath.Join("testdata", "rar5-compressed", "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS."+n+".rar"))
		if err != nil {
			t.Fatal(err)
		}
		if i == 1 {
			b[len(b)/2] ^= 0xff
		}
		if err := os.WriteFile(filepath.Join(dir, "set."+n+".rar"), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := ExtractCompressed(context.Background(), filepath.Join(dir, "set.part01.rar"), t.TempDir(), rar5Size); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("got %v, want ErrCorrupt", err)
	}
}

// TestExtractCompressed_ContextEnded stops when the context has ended.
func TestExtractCompressed_ContextEnded(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ExtractCompressed(ctx, rar5First(), t.TempDir(), rar5Size); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
}
