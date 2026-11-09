package library

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// An adopted .m4a is named by its codec, so an ALAC library is known
// as lossless and an AAC one as lossy; other files by their extension.
func TestAdoptedFormatReadsTheCodec(t *testing.T) {
	box := func(kind string, payload []byte) []byte {
		b := make([]byte, 8+len(payload))
		binary.BigEndian.PutUint32(b[:4], uint32(len(b)))
		copy(b[4:8], kind)
		copy(b[8:], payload)
		return b
	}
	m4a := func(codec string) []byte {
		hdlr := append(append([]byte{0, 0, 0, 0, 0, 0, 0, 0}, []byte("soun")...), make([]byte, 12)...)
		entry := make([]byte, 16)
		binary.BigEndian.PutUint32(entry[:4], 16)
		copy(entry[4:8], codec)
		stsd := append([]byte{0, 0, 0, 0, 0, 0, 0, 1}, entry...)
		mdia := append(box("mdhd", make([]byte, 24)), box("hdlr", hdlr)...)
		mdia = append(mdia, box("minf", box("stbl", box("stsd", stsd)))...)
		return append(box("ftyp", []byte("M4A \x00\x00\x00\x00M4A ")), box("moov", box("trak", box("mdia", mdia)))...)
	}
	dir := t.TempDir()
	for name, c := range map[string]struct {
		data []byte
		want string
	}{
		"alac.m4a": {m4a("alac"), "ALAC"},
		"aac.m4a":  {m4a("mp4a"), "AAC"},
		"odd.m4a":  {[]byte("not a file"), "M4A"},
		"x.flac":   {[]byte("fLaC"), "FLAC"},
	} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, c.data, 0o644); err != nil {
			t.Fatal(err)
		}
		if got := adoptedFormat(p, filepath.Ext(name)); got != c.want {
			t.Errorf("%s: %s, want %s", name, got, c.want)
		}
	}
}
