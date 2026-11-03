package unrar

import (
	"context"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"
)

// RAR 4 sets from the rarfile project (testdata/rarfile/README.md); sizes and
// CRCs from its own listings (the .exp files beside them).
type rar4File struct {
	name string
	size int64
	crc  uint32
}

var (
	rar4Old   = []rar4File{{"vols/bigfile.txt", 205000, 0x509ad74c}, {"vols/smallfile.txt", 2050, 0xd08a1f86}}
	rar4Solid = []rar4File{{"stest1.txt", 2048, 0xc5b7e6a2}, {"stest2.txt", 2048, 0xc5b7e6a2}}
)

func rar4Path(name string) string { return filepath.Join("testdata", "rarfile", name) }

func checkRar4(t *testing.T, dest string, want []rar4File) {
	t.Helper()
	for _, w := range want {
		b, err := os.ReadFile(filepath.Join(dest, filepath.FromSlash(w.name)))
		if err != nil {
			t.Fatalf("%s: %v", w.name, err)
		}
		if int64(len(b)) != w.size || crc32.ChecksumIEEE(b) != w.crc {
			t.Fatalf("%s: %d bytes, crc %08x; want %d, %08x", w.name, len(b), crc32.ChecksumIEEE(b), w.size, w.crc)
		}
	}
}

// A stored RAR 4 set over .rar, .r00 and .r01: the built-in reader takes it.
func TestRAR4OldNamingSetIsReadAndUnpacked(t *testing.T) {
	es, err := List(rar4Path("rar3-old.rar"))
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 2 || es[0].Name != rar4Old[0].name || es[0].Size != rar4Old[0].size || es[0].Compressed || es[1].Name != rar4Old[1].name {
		t.Fatalf("entries %+v", es)
	}
	dest := t.TempDir()
	if _, err := Extract(rar4Path("rar3-old.rar"), dest); err != nil {
		t.Fatal(err)
	}
	checkRar4(t, dest, rar4Old)
	// The decoder reads the same set too.
	dest2 := t.TempDir()
	if err := ExtractCompressed(context.Background(), rar4Path("rar3-old.rar"), dest2, 1<<20); err != nil {
		t.Fatal(err)
	}
	checkRar4(t, dest2, rar4Old)
}

// A compressed, solid RAR 4 (2.9) archive: listed as compressed, refused by
// the stored-set reader, unpacked by the built-in decoder.
func TestRAR4CompressedSolidArchiveIsDecoded(t *testing.T) {
	es, err := List(rar4Path("rar3-solid.rar"))
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 2 || !es[0].Compressed || es[0].Size != 2048 {
		t.Fatalf("entries %+v", es)
	}
	dest := t.TempDir()
	if err := ExtractCompressed(context.Background(), rar4Path("rar3-solid.rar"), dest, 4096); err != nil {
		t.Fatal(err)
	}
	checkRar4(t, dest, rar4Solid)
}
