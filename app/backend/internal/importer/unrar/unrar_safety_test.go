package unrar

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestExtract_WrongChecksum reports a wrong data checksum as damage.
func TestExtract_WrongChecksum(t *testing.T) {
	for name, vol := range formats() {
		t.Run(name, func(t *testing.T) {
			dir, dest := t.TempDir(), t.TempDir()
			first := put(t, dir, "rel.rar", vol(false, spec{name: "movie.mkv", data: movie, crc: 12345}))
			if _, err := List(first); err != nil {
				t.Fatalf("listing reads headers only: %v", err)
			}
			if _, err := Extract(first, dest); !errors.Is(err, ErrCorrupt) {
				t.Fatalf("got %v", err)
			}
			good := vol(false, spec{name: "movie.mkv", data: movie})
			good[bytes.Index(good, movie[:20])+3] ^= 0xFF
			bad := put(t, dir, "flip.rar", good)
			if _, err := Extract(bad, t.TempDir()); !errors.Is(err, ErrCorrupt) {
				t.Fatalf("flipped byte: %v", err)
			}
		})
	}
}

// TestExtract_LinksAndSpecialFiles refuses links and special files.
func TestExtract_LinksAndSpecialFiles(t *testing.T) {
	for name, vol := range formats() {
		t.Run(name, func(t *testing.T) {
			dir, dest := t.TempDir(), t.TempDir()
			first := put(t, dir, "l.rar", vol(false, spec{name: "movie.mkv", data: movie}, spec{name: "link.mkv", data: []byte("/etc/passwd"), unix: 0xA1FF}))
			es, err := List(first)
			if err != nil || len(es) != 2 || !es[1].Link || es[0].Link {
				t.Fatalf("%+v %v", es, err)
			}
			if _, err := Extract(first, dest); !errors.Is(err, ErrUnrecognised) {
				t.Fatalf("link: %v", err)
			}
			fifo := put(t, dir, "f.rar", vol(false, spec{name: "pipe", unix: 0x11A4}))
			if _, err := List(fifo); !errors.Is(err, ErrUnrecognised) {
				t.Fatalf("fifo: %v", err)
			}
			if left, _ := os.ReadDir(dest); len(left) != 0 {
				t.Fatal("wrote before refusing")
			}
		})
	}
}

// TestList_UnknownBlock refuses blocks it does not know.
func TestList_UnknownBlock(t *testing.T) {
	dir := t.TempDir()
	v4 := rar4Volume(false, spec{name: "a.mkv", data: movie})
	v4 = append(append(append([]byte{}, v4[:7]...), rar4Block(0x7F, 0, []byte{1, 2})...), v4[7:]...)
	if _, err := List(put(t, dir, "a.rar", v4)); !errors.Is(err, ErrUnrecognised) {
		t.Fatalf("rar4: %v", err)
	}
	v5 := rar5Volume(false, spec{name: "a.mkv", data: movie})
	v5 = append(append(append([]byte{}, v5[:8]...), rar5Block(9, 0, []byte{1}, nil, 0)...), v5[8:]...)
	if _, err := List(put(t, dir, "b.rar", v5)); !errors.Is(err, ErrUnrecognised) {
		t.Fatalf("rar5 block: %v", err)
	}
	extra := rar5Volume(false, spec{name: "a.mkv", data: movie, extraType: 77})
	if _, err := List(put(t, dir, "c.rar", extra)); !errors.Is(err, ErrUnrecognised) {
		t.Fatalf("rar5 record: %v", err)
	}
}

func unicodeName(plain, uni string) []byte {
	out := append([]byte(plain), 0, 0)
	for i := 0; i < len(uni); i += 4 {
		out = append(out, 0)
		out = append(out, uni[i:min(i+4, len(uni))]...)
	}
	return out
}

// TestList_RAR4NamesDisagree refuses a RAR 4 entry whose two name forms differ.
func TestList_RAR4NamesDisagree(t *testing.T) {
	dir := t.TempDir()
	ok := put(t, dir, "ok.rar", rar4Volume(false, spec{name: "movie.mkv", data: movie, rawName: unicodeName("movie.mkv", "movie.mkv")}))
	es, err := List(ok)
	if err != nil || len(es) != 1 || es[0].Name != "movie.mkv" {
		t.Fatalf("%+v %v", es, err)
	}
	differ := put(t, dir, "differ.rar", rar4Volume(false, spec{name: "x", data: movie, rawName: unicodeName("abcd.mkv", "abce.mkv")}))
	if _, err := List(differ); !errors.Is(err, ErrUnrecognised) {
		t.Fatalf("differing names: %v", err)
	}
	evil := put(t, dir, "evil.rar", rar4Volume(false, spec{name: "x", data: movie, rawName: unicodeName("..x/movie", "../movie")}))
	if _, err := List(evil); err == nil {
		t.Fatal("escaping name accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, "..", "movie")); err == nil {
		t.Fatal("wrote outside")
	}
}

// TestList_BigServiceBlock refuses a service block over the size cap.
func TestList_BigServiceBlock(t *testing.T) {
	dir := t.TempDir()
	sub4 := func(n int) []byte {
		body := make([]byte, 4, 4+8)
		binary.LittleEndian.PutUint32(body, uint32(n))
		body = append(body, []byte("CMTxxxxx")...)
		return append(rar4Block(0x7A, 0x8000, body), make([]byte, n)...)
	}
	v4 := rar4Volume(false, spec{name: "a.mkv", data: movie})
	small := append(append(append([]byte{}, v4[:7]...), sub4(64)...), v4[7:]...)
	if _, err := List(put(t, dir, "a.rar", small)); err != nil {
		t.Fatalf("rar4 small comment: %v", err)
	}
	big := append(append(append([]byte{}, v4[:7]...), sub4(maxServiceData+1)...), v4[7:]...)
	if _, err := List(put(t, dir, "b.rar", big)); !errors.Is(err, ErrUnrecognised) {
		t.Fatalf("rar4 big stream: %v", err)
	}
	v5 := rar5Volume(false, spec{name: "a.mkv", data: movie})
	sub5 := func(n int) []byte {
		return append(rar5Block(3, 0, []byte{0, 0, 0, 3, 'S', 'T', 'M'}, nil, n), make([]byte, n)...)
	}
	small5 := append(append(append([]byte{}, v5[:8]...), sub5(64)...), v5[8:]...)
	if _, err := List(put(t, dir, "c.rar", small5)); err != nil {
		t.Fatalf("rar5 small service: %v", err)
	}
	big5 := append(append(append([]byte{}, v5[:8]...), sub5(maxServiceData+1)...), v5[8:]...)
	if _, err := List(put(t, dir, "d.rar", big5)); !errors.Is(err, ErrUnrecognised) {
		t.Fatalf("rar5 big stream: %v", err)
	}
}
