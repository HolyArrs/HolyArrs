package unrar

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"
)

type spec struct {
	name                    string
	data                    []byte
	splitBefore, splitAfter bool
	compressed, encrypted   bool
	total                   int
	crc                     uint32
	unix                    uint32
	rawName                 []byte
	extraType               uint64
}

func (f spec) sum() uint32 {
	if f.crc != 0 {
		return f.crc
	}
	return crc32.ChecksumIEEE(f.data)
}

func rar4Block(typ byte, flags uint16, body []byte) []byte {
	hsize := 7 + len(body)
	b := make([]byte, 7, hsize)
	b[2] = typ
	binary.LittleEndian.PutUint16(b[3:], flags)
	binary.LittleEndian.PutUint16(b[5:], uint16(hsize))
	b = append(b, body...)
	binary.LittleEndian.PutUint16(b[0:], uint16(crc32.ChecksumIEEE(b[2:])))
	return b
}

func rar4Volume(more bool, files ...spec) []byte {
	out := append([]byte{}, sigRar4...)
	out = append(out, rar4Block(0x73, 0x0001, make([]byte, 6))...)
	for _, f := range files {
		var fl uint16 = 0x8000
		if f.splitBefore {
			fl |= 1
		}
		if f.splitAfter {
			fl |= 2
		}
		if f.encrypted {
			fl |= 4
		}
		total := f.total
		if total == 0 {
			total = len(f.data)
		}
		nameField := []byte(f.name)
		if f.rawName != nil {
			nameField = f.rawName
			fl |= 0x200
		}
		body := make([]byte, 25+len(nameField))
		binary.LittleEndian.PutUint32(body[0:], uint32(len(f.data)))
		binary.LittleEndian.PutUint32(body[4:], uint32(total))
		body[8] = 2
		if f.unix != 0 {
			body[8] = 3
			binary.LittleEndian.PutUint32(body[21:], f.unix)
		}
		binary.LittleEndian.PutUint32(body[9:], f.sum())
		body[17] = 29
		body[18] = 0x30
		if f.compressed {
			body[18] = 0x33
		}
		binary.LittleEndian.PutUint16(body[19:], uint16(len(nameField)))
		copy(body[25:], nameField)
		out = append(out, rar4Block(0x74, fl, body)...)
		out = append(out, f.data...)
	}
	var endFlags uint16
	if more {
		endFlags = 1
	}
	return append(out, rar4Block(0x7B, endFlags, nil)...)
}

func vint(v uint64) []byte {
	var b []byte
	for v >= 0x80 {
		b = append(b, byte(v)|0x80)
		v >>= 7
	}
	return append(b, byte(v))
}

func rar5Block(typ uint64, flags uint64, fields []byte, extra []byte, dataSize int) []byte {
	var h []byte
	h = append(h, vint(typ)...)
	if len(extra) > 0 {
		flags |= 1
	}
	if dataSize > 0 {
		flags |= 2
	}
	h = append(h, vint(flags)...)
	if len(extra) > 0 {
		h = append(h, vint(uint64(len(extra)))...)
	}
	if dataSize > 0 {
		h = append(h, vint(uint64(dataSize))...)
	}
	h = append(h, fields...)
	h = append(h, extra...)
	body := append(vint(uint64(len(h))), h...)
	out := make([]byte, 4, 4+len(body))
	binary.LittleEndian.PutUint32(out, crc32.ChecksumIEEE(body))
	return append(out, body...)
}

func rar5Volume(more bool, files ...spec) []byte {
	out := append([]byte{}, sigRar5...)
	main := vint(0x1)
	out = append(out, rar5Block(1, 0, main, nil, 0)...)
	for _, f := range files {
		var fl uint64
		if f.splitBefore {
			fl |= 0x8
		}
		if f.splitAfter {
			fl |= 0x10
		}
		total := f.total
		if total == 0 {
			total = len(f.data)
		}
		var fields []byte
		fields = append(fields, vint(4)...)
		fields = append(fields, vint(uint64(total))...)
		attr, host := uint64(0x20), uint64(0)
		if f.unix != 0 {
			attr, host = uint64(f.unix), 1
		}
		fields = append(fields, vint(attr)...)
		fields = binary.LittleEndian.AppendUint32(fields, f.sum())
		comp := uint64(0)
		if f.compressed {
			comp = 3 << 7
		}
		fields = append(fields, vint(comp)...)
		fields = append(fields, vint(host)...)
		fields = append(fields, vint(uint64(len(f.name)))...)
		fields = append(fields, f.name...)
		var extra []byte
		if f.encrypted {
			extra = append(vint(1), vint(1)...)
		}
		if f.extraType != 0 {
			extra = append(vint(1), vint(f.extraType)...)
		}
		out = append(out, rar5Block(2, fl, fields, extra, len(f.data))...)
		out = append(out, f.data...)
	}
	var endFlags uint64
	if more {
		endFlags = 1
	}
	return append(out, rar5Block(5, 0, vint(endFlags), nil, 0)...)
}

func put(t *testing.T, dir, name string, b []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

var movie = bytes.Repeat([]byte("moviebytes-"), 500)

func formats() map[string]func(bool, ...spec) []byte {
	return map[string]func(bool, ...spec) []byte{"rar4": rar4Volume, "rar5": rar5Volume}
}

// TestExtract_SingleVolume unpacks one stored volume.
func TestExtract_SingleVolume(t *testing.T) {
	for name, vol := range formats() {
		t.Run(name, func(t *testing.T) {
			dir, dest := t.TempDir(), t.TempDir()
			first := put(t, dir, "rel.rar", vol(false, spec{name: "Rel/movie.mkv", data: movie}, spec{name: `Rel\info.nfo`, data: []byte("hi")}))
			es, err := List(first)
			if err != nil || len(es) != 2 || es[0].Name != "Rel/movie.mkv" || es[0].Size != int64(len(movie)) || es[1].Name != "Rel/info.nfo" {
				t.Fatalf("list %+v %v", es, err)
			}
			if _, err := Extract(first, dest); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(filepath.Join(dest, "Rel", "movie.mkv"))
			if err != nil || !bytes.Equal(got, movie) {
				t.Fatalf("content differs: %v", err)
			}
		})
	}
}

// TestExtract_MultiVolume unpacks a stored set split over volumes.
func TestExtract_MultiVolume(t *testing.T) {
	a, b, c := movie[:2000], movie[2000:4000], movie[4000:]
	for _, c1 := range []struct {
		style string
		names [3]string
	}{
		{"old", [3]string{"rel.rar", "rel.r00", "rel.r01"}},
		{"part", [3]string{"rel.part01.rar", "rel.part02.rar", "rel.part03.rar"}},
	} {
		for name, vol := range formats() {
			t.Run(c1.style+"/"+name, func(t *testing.T) {
				dir, dest := t.TempDir(), t.TempDir()
				put(t, dir, c1.names[0], vol(true, spec{name: "movie.mkv", data: a, splitAfter: true, total: len(movie)}))
				put(t, dir, c1.names[1], vol(true, spec{name: "movie.mkv", data: b, splitBefore: true, splitAfter: true, total: len(movie)}))
				put(t, dir, c1.names[2], vol(false, spec{name: "movie.mkv", data: c, splitBefore: true, total: len(movie), crc: crc32.ChecksumIEEE(movie)}, spec{name: "after.txt", data: []byte("x")}))
				es, err := Extract(filepath.Join(dir, c1.names[0]), dest)
				if err != nil || len(es) != 2 || es[0].Size != int64(len(movie)) {
					t.Fatalf("%+v %v", es, err)
				}
				got, _ := os.ReadFile(filepath.Join(dest, "movie.mkv"))
				if !bytes.Equal(got, movie) {
					t.Fatal("content differs")
				}
			})
		}
	}
}

// TestExtract_MissingOrCutShort reports a missing or cut volume.
func TestExtract_MissingOrCutShort(t *testing.T) {
	for name, vol := range formats() {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			first := put(t, dir, "rel.rar", vol(true, spec{name: "movie.mkv", data: movie[:100], splitAfter: true, total: len(movie)}))
			if _, err := List(first); !errors.Is(err, ErrCorrupt) {
				t.Fatalf("missing volume: %v", err)
			}
			whole := vol(false, spec{name: "movie.mkv", data: movie})
			cut := put(t, dir, "cut.rar", whole[:len(whole)-200])
			if _, err := List(cut); !errors.Is(err, ErrCorrupt) {
				t.Fatalf("cut short: %v", err)
			}
			put(t, dir, "junk.rar", []byte("not a rar file at all"))
			if _, err := List(filepath.Join(dir, "junk.rar")); !errors.Is(err, ErrCorrupt) {
				t.Fatalf("junk: %v", err)
			}
		})
	}
}

// TestExtract_UnsafePaths refuses unsafe paths and writes nothing.
func TestExtract_UnsafePaths(t *testing.T) {
	for name, vol := range formats() {
		for _, bad := range []string{"../evil.mkv", "a/../../evil.mkv", "/etc/evil.mkv", `..\evil.mkv`, `C:\evil.mkv`} {
			t.Run(name+"/"+bad, func(t *testing.T) {
				dir := t.TempDir()
				dest := filepath.Join(t.TempDir(), "out")
				if err := os.Mkdir(dest, 0o755); err != nil {
					t.Fatal(err)
				}
				first := put(t, dir, "rel.rar", vol(false, spec{name: "ok.mkv", data: []byte("fine")}, spec{name: bad, data: []byte("bad")}))
				if _, err := Extract(first, dest); !errors.Is(err, ErrUnsafePath) {
					t.Fatalf("got %v", err)
				}
				if _, err := os.Stat(filepath.Join(filepath.Dir(dest), "evil.mkv")); err == nil {
					t.Fatal("wrote outside")
				}
			})
		}
	}
}

// TestList_CompressedAndEncrypted reports compressed and encrypted entries.
func TestList_CompressedAndEncrypted(t *testing.T) {
	for name, vol := range formats() {
		t.Run(name, func(t *testing.T) {
			dir, dest := t.TempDir(), t.TempDir()
			first := put(t, dir, "c.rar", vol(false, spec{name: "movie.mkv", data: []byte("packed"), compressed: true, total: 9999}))
			es, err := List(first)
			if err != nil || len(es) != 1 || !es[0].Compressed || es[0].Size != 9999 {
				t.Fatalf("%+v %v", es, err)
			}
			if _, err := Extract(first, dest); !errors.Is(err, ErrCompressed) {
				t.Fatalf("got %v", err)
			}
			if left, _ := os.ReadDir(dest); len(left) != 0 {
				t.Fatal("wrote before refusing")
			}
			enc := put(t, dir, "e.rar", vol(false, spec{name: "movie.mkv", data: []byte("secret"), encrypted: true}))
			if _, err := Extract(enc, dest); !errors.Is(err, ErrEncrypted) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

// TestNextVolume_Names checks the volume name patterns.
func TestNextVolume_Names(t *testing.T) {
	for in, want := range map[string]string{
		"a/rel.rar": "a/rel.r00", "rel.r00": "rel.r01", "rel.r99": "rel.s00", "REL.RAR": "REL.R00",
		"rel.part01.rar": "rel.part02.rar", "rel.part9.rar": "rel.part10.rar", "rel.part099.rar": "rel.part100.rar", "x.mkv": "",
	} {
		if got := NextVolume(filepath.FromSlash(in)); got != filepath.FromSlash(want) {
			t.Errorf("%s: %q want %q", in, got, want)
		}
	}
	if !IsFirstVolume("a.rar") || !IsFirstVolume("a.part01.rar") || IsFirstVolume("a.part02.rar") || IsFirstVolume("a.r00") {
		t.Error("first volume detection")
	}
	if !IsVolume("a.r07") || !IsVolume("A.RAR") || IsVolume("a.mkv") {
		t.Error("volume detection")
	}
}

// TestStoredEntryStaysInDest refuses to follow a folder link out of dest.
func TestStoredEntryStaysInDest(t *testing.T) {
	for name, vol := range formats() {
		t.Run(name, func(t *testing.T) {
			dir, dest, outside := t.TempDir(), t.TempDir(), t.TempDir()
			if err := os.Symlink(outside, filepath.Join(dest, "sub")); err != nil {
				t.Skipf("no symlinks here: %v", err)
			}
			first := put(t, dir, "rel.rar", vol(false, spec{name: "sub/a.mkv", data: movie}))
			if _, err := Extract(first, dest); err == nil {
				t.Fatal("want an error")
			}
			if left, _ := os.ReadDir(outside); len(left) != 0 {
				t.Fatalf("wrote outside: %v", left)
			}
		})
	}
}
