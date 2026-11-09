package tags

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func ss(n int) []byte {
	return []byte{byte(n >> 21 & 0x7F), byte(n >> 14 & 0x7F), byte(n >> 7 & 0x7F), byte(n & 0x7F)}
}

// id3 builds an ID3v2 tag of the given version from text frames.
func id3(ver byte, frames ...[2]string) []byte {
	var body bytes.Buffer
	for _, f := range frames {
		data := append([]byte{3}, f[1]...)
		switch ver {
		case 2:
			body.WriteString(f[0])
			body.Write([]byte{byte(len(data) >> 16), byte(len(data) >> 8), byte(len(data))})
		case 3:
			body.WriteString(f[0])
			_ = binary.Write(&body, binary.BigEndian, uint32(len(data)))
			body.Write([]byte{0, 0})
		default:
			body.WriteString(f[0])
			body.Write(ss(len(data)))
			body.Write([]byte{0, 0})
		}
		body.Write(data)
	}
	body.Write(make([]byte, 16)) // padding
	out := append([]byte("ID3"), ver, 0, 0)
	out = append(out, ss(body.Len())...)
	return append(out, body.Bytes()...)
}

// flac builds a FLAC header with a picture block and a comment block.
func flac(comments ...string) []byte {
	var vc bytes.Buffer
	_ = binary.Write(&vc, binary.LittleEndian, uint32(4))
	vc.WriteString("test")
	_ = binary.Write(&vc, binary.LittleEndian, uint32(len(comments)))
	for _, c := range comments {
		_ = binary.Write(&vc, binary.LittleEndian, uint32(len(c)))
		vc.WriteString(c)
	}
	out := []byte("fLaC")
	out = append(out, 0, 0, 0, 34) // STREAMINFO
	out = append(out, make([]byte, 34)...)
	pic := make([]byte, 5000)
	out = append(out, 6, byte(len(pic)>>16), byte(len(pic)>>8), byte(len(pic)))
	out = append(out, pic...)
	n := vc.Len()
	out = append(out, 0x80|4, byte(n>>16), byte(n>>8), byte(n))
	return append(out, vc.Bytes()...)
}

func TestID3Versions(t *testing.T) {
	for _, c := range []struct {
		ver    byte
		frames [][2]string
	}{
		{2, [][2]string{{"TT2", "Tide"}, {"TRK", "3/12"}, {"TPA", "2/2"}, {"TP1", "Lab Lanterns"}, {"TAL", "Small Hours"}}},
		{3, [][2]string{{"TIT2", "Tide"}, {"TRCK", "3/12"}, {"TPOS", "2"}, {"TPE1", "Lab Lanterns"}, {"TALB", "Small Hours"}}},
		{4, [][2]string{{"TIT2", "Tide"}, {"TRCK", "03"}, {"TPOS", "2/2"}, {"TPE1", "Lab Lanterns"}, {"TALB", "Small Hours"}}},
	} {
		got, err := Read(bytes.NewReader(append(id3(c.ver, c.frames...), 0xFF, 0xFB)))
		want := Tags{Title: "Tide", Artist: "Lab Lanterns", Album: "Small Hours", Track: 3, Disc: 2}
		if err != nil || got != want {
			t.Errorf("v2.%d: %+v %v", c.ver, got, err)
		}
	}
}

func TestID3Encodings(t *testing.T) {
	latin := append([]byte{0}, 'C', 0xE9, 'u')
	utf16bom := []byte{1, 0xFF, 0xFE, 'D', 0, 0xE9, 0, 0, 0}
	utf16be := []byte{2, 0, 'Z', 0x00, 0xFC}
	for _, c := range []struct {
		body []byte
		want string
	}{{latin, "Céu"}, {utf16bom, "Dé"}, {utf16be, "Zü"}} {
		if got := id3Text(c.body); got != c.want {
			t.Errorf("got %q want %q", got, c.want)
		}
	}
}

func TestFLAC(t *testing.T) {
	got, err := Read(bytes.NewReader(flac("TITLE=Tide", "tracknumber=4", "DISCNUMBER=1/2", "ARTIST=Lab Lanterns", "ALBUM=Small Hours", "junk")))
	if err != nil || got != (Tags{Title: "Tide", Artist: "Lab Lanterns", Album: "Small Hours", Track: 4, Disc: 1}) {
		t.Fatalf("%+v %v", got, err)
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "a.flac")
	if err := os.WriteFile(p, flac("TITLE=From disk"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadFile(p); err != nil || got.Title != "From disk" {
		t.Fatalf("file: %+v %v", got, err)
	}
}

func TestNoTags(t *testing.T) {
	for name, b := range map[string][]byte{
		"empty":          nil,
		"plain mp3":      {0xFF, 0xFB, 0x90, 0x00},
		"wav":            []byte("RIFF\x00\x00\x00\x00WAVE"),
		"bad id3 size":   append([]byte("ID3\x03\x00\x00"), 0x80, 0, 0, 0),
		"id3 version 9":  append([]byte("ID3\x09\x00\x00"), 0, 0, 0, 10),
		"flac no blocks": []byte("fLaC"),
		"flac bad block": append([]byte("fLaC"), 0x7F, 0, 0, 0),
	} {
		if got, err := Read(bytes.NewReader(b)); err == nil {
			t.Errorf("%s: %+v", name, got)
		}
	}
}

// TestHostileInput: huge declared sizes, counts and frames running past the
// end are read within the limits and give no tags or only what fits.
func TestHostileInput(t *testing.T) {
	// A comment block claiming 16 MB.
	b := append([]byte("fLaC"), 0x84, 0xFF, 0xFF, 0xFF)
	if _, err := Read(bytes.NewReader(b)); err == nil {
		t.Error("an oversize comment block was read")
	}
	// A comment count of four billion with one real comment.
	var vc bytes.Buffer
	_ = binary.Write(&vc, binary.LittleEndian, uint32(0))
	_ = binary.Write(&vc, binary.LittleEndian, uint32(0xFFFFFFFF))
	_ = binary.Write(&vc, binary.LittleEndian, uint32(7))
	vc.WriteString("TITLE=x")
	_ = binary.Write(&vc, binary.LittleEndian, uint32(0xFFFFFFF0))
	b = append([]byte("fLaC"), 0x84, 0, byte(vc.Len()>>8), byte(vc.Len()))
	b = append(b, vc.Bytes()...)
	if got, err := Read(bytes.NewReader(b)); err != nil || got.Title != "x" {
		t.Errorf("count: %+v %v", got, err)
	}
	// An ID3 tag claiming 256 MB with a frame running past its end.
	tag := append([]byte("ID3\x03\x00\x00"), 0x7F, 0x7F, 0x7F, 0x7F)
	tag = append(tag, "TIT2"...)
	tag = append(tag, 0x7F, 0xFF, 0xFF, 0xFF, 0, 0, 3, 'a')
	if got, err := Read(bytes.NewReader(tag)); err == nil {
		t.Errorf("frame past the end: %+v", got)
	}
	// A very long value is cut on a character boundary.
	long := strings.Repeat("é", 5000)
	got, err := Read(bytes.NewReader(id3(3, [2]string{"TIT2", long})))
	if err != nil || len(got.Title) > maxValueBytes || !utf8.ValidString(got.Title) {
		t.Errorf("long value: %d bytes, %v", len(got.Title), err)
	}
	// Control characters and broken UTF-8 are dropped; odd numbers are 0.
	got, _ = Read(bytes.NewReader(id3(4, [2]string{"TIT2", "a\x01b\xffc"}, [2]string{"TRCK", "-3"}, [2]string{"TPOS", "99999"})))
	if got.Title != "abc" || got.Track != 0 || got.Disc != 0 {
		t.Errorf("clean: %+v", got)
	}
	// Many tiny frames stop at the frame limit quickly.
	frames := make([][2]string, 5000)
	for i := range frames {
		frames[i] = [2]string{"TXXX", "x"}
	}
	start := time.Now()
	_, _ = Read(bytes.NewReader(id3(3, frames...)))
	if time.Since(start) > 2*time.Second {
		t.Error("too slow")
	}
}

func FuzzRead(f *testing.F) {
	f.Add(id3(2, [2]string{"TT2", "a"}))
	f.Add(id3(3, [2]string{"TIT2", "a"}, [2]string{"TRCK", "1/2"}))
	f.Add(id3(4, [2]string{"TPOS", "2"}))
	f.Add(flac("TITLE=a", "TRACKNUMBER=2"))
	f.Add([]byte("fLaC\x84\x00\x00\x08\x00\x00\x00\x00\xff\xff\xff\xff"))
	f.Fuzz(func(t *testing.T, b []byte) {
		got, _ := Read(bytes.NewReader(b))
		for _, v := range []string{got.Title, got.Artist, got.Album} {
			if len(v) > maxValueBytes || !utf8.ValidString(v) {
				t.Fatalf("bad value %q", v)
			}
		}
		if got.Track < 0 || got.Track > 999 || got.Disc < 0 || got.Disc > 999 {
			t.Fatalf("bad number %+v", got)
		}
	})
}

// An ID3v2.2 tag with the compression flag set has no frames
// this package can read.
func TestID3v22Compressed(t *testing.T) {
	b := id3(2, [2]string{"TT2", "Tide"})
	b[5] |= 0x40
	if got, err := Read(bytes.NewReader(b)); err != ErrNoTags {
		t.Fatalf("%+v %v", got, err)
	}
}

// ReadFile reads only regular files; a folder or a link is
// refused before it is opened.
func TestReadFileRegularOnly(t *testing.T) {
	dir := t.TempDir()
	if _, err := ReadFile(dir); err == nil {
		t.Fatal("a folder was read")
	}
	f := filepath.Join(dir, "a.mp3")
	if err := os.WriteFile(f, id3(4, [2]string{"TIT2", "Tide"}), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadFile(f); err != nil || got.Title != "Tide" {
		t.Fatalf("%+v %v", got, err)
	}
	link := filepath.Join(dir, "link.mp3")
	if os.Symlink(f, link) == nil {
		if _, err := ReadFile(link); err == nil {
			t.Fatal("a link was followed")
		}
	}
}
