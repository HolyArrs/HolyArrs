package sites

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"testing"
)

// tlRar builds a stored (uncompressed) RAR4 archive holding the given files.
func tlRar(names []string, datas [][]byte) []byte {
	block := func(typ byte, flags uint16, body []byte) []byte {
		b := make([]byte, 7, 7+len(body))
		b[2] = typ
		binary.LittleEndian.PutUint16(b[3:], flags)
		binary.LittleEndian.PutUint16(b[5:], uint16(7+len(body)))
		b = append(b, body...)
		binary.LittleEndian.PutUint16(b[0:], uint16(crc32.ChecksumIEEE(b[2:])))
		return b
	}
	out := []byte("Rar!\x1a\x07\x00")
	out = append(out, block(0x73, 0, make([]byte, 6))...)
	for i, name := range names {
		data := datas[i]
		body := make([]byte, 25+len(name))
		binary.LittleEndian.PutUint32(body[0:], uint32(len(data)))
		binary.LittleEndian.PutUint32(body[4:], uint32(len(data)))
		body[8], body[17], body[18] = 2, 29, 0x30
		binary.LittleEndian.PutUint32(body[9:], crc32.ChecksumIEEE(data))
		binary.LittleEndian.PutUint16(body[19:], uint16(len(name)))
		copy(body[25:], name)
		out = append(out, block(0x74, 0x8000, body)...)
		out = append(out, data...)
	}
	return append(out, block(0x7B, 0, nil)...)
}

func TestTitloviRarRepackCaps(t *testing.T) {
	many := func(n, size int) ([]string, [][]byte) {
		var ns []string
		var ds [][]byte
		for i := 0; i < n; i++ {
			ns = append(ns, fmt.Sprintf("Sub.%03d.srt", i))
			ds = append(ds, bytes.Repeat([]byte{'a' + byte(i%26)}, size))
		}
		return ns, ds
	}
	cases := []struct {
		name    string
		n, size int
		wantErr bool
		files   int
	}{
		{"small rar works", 2, 100, false, 2},
		{"200 files is fine", 200, 10, false, 200},
		{"201 tiny files", 201, 10, true, 0},
		{"over 20 MB in all", 3, 8 << 20, true, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ns, ds := many(c.n, c.size)
			z, err := tlAsZip(tlRar(ns, ds))
			if c.wantErr {
				var e *Error
				if !errors.As(err, &e) || e.Kind != KindUnreadable {
					t.Fatalf("want an unreadable error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			zr, err := zip.NewReader(bytes.NewReader(z), int64(len(z)))
			if err != nil {
				t.Fatal(err)
			}
			if len(zr.File) != c.files {
				t.Fatalf("got %d files, want %d", len(zr.File), c.files)
			}
		})
	}
}
