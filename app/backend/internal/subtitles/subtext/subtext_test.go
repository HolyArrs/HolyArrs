package subtext

import (
	"archive/zip"
	"bytes"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"time"
)

const srt = "1\r\n00:00:01,000 --> 00:00:02,500\r\nHello\r\n\r\n2\r\n00:01:00,250 --> 00:01:02,000 X1:10\r\nAgain, 00:00:09,000 --> no\r\n"

func TestRetimeSRT(t *testing.T) {
	got, err := Retime(srt, SRT, Fix{OffsetMS: 1500})
	if err != nil {
		t.Fatal(err)
	}
	want := "1\r\n00:00:02,500 --> 00:00:04,000\r\nHello\r\n\r\n2\r\n00:01:01,750 --> 00:01:03,500 X1:10\r\nAgain, 00:00:09,000 --> no\r\n"
	if got != want {
		t.Fatalf("got %q", got)
	}
	// Shifting before zero stops at zero.
	got, _ = Retime(srt, SRT, Fix{OffsetMS: -2000})
	if !strings.Contains(got, "00:00:00,000 --> 00:00:00,500") {
		t.Fatalf("negative: %q", got)
	}
	// A 25 fps subtitle on a 23.976 fps video must play later.
	got, _ = Retime("1\n01:00:00,000 --> 01:00:01,000\nx\n", SRT, Fix{FromFPS: 25, ToFPS: 23.976})
	if !strings.HasPrefix(got, "1\n01:02:33,754 --> 01:02:34,796") {
		t.Fatalf("fps: %q", got)
	}
}

func TestRetimeVTTAndASS(t *testing.T) {
	vtt := "WEBVTT\n\n00:01.000 --> 00:02.000 align:start\nhi\n"
	got, err := Retime(vtt, VTT, Fix{OffsetMS: 1000})
	if err != nil || !strings.Contains(got, "00:00:02.000 --> 00:00:03.000 align:start") {
		t.Fatalf("vtt: %q %v", got, err)
	}
	ass := "[Script Info]\nScriptType: v4.00+\n\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n" +
		"Dialogue: 0,0:00:01.50,0:00:03.00,Default,,0,0,0,,Hello, world, 0:00:09.00\n"
	got, err = Retime(ass, ASS, Fix{OffsetMS: 500})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Dialogue: 0,0:00:02.00,0:00:03.50,Default,,0,0,0,,Hello, world, 0:00:09.00\n") {
		t.Fatalf("ass: %q", got)
	}
	// Fields in a different order.
	odd := "[Events]\nFormat: Start, End, Text\nDialogue: 0:00:01.00,0:00:02.00,x\n"
	got, _ = Retime(odd, SSA, Fix{OffsetMS: 1000})
	if !strings.Contains(got, "Dialogue: 0:00:02.00,0:00:03.00,x") {
		t.Fatalf("ssa order: %q", got)
	}
}

func TestRetimeRefusals(t *testing.T) {
	for _, f := range []Fix{{}, {OffsetMS: MaxOffsetMS + 1}, {FromFPS: 25}, {FromFPS: 0.5, ToFPS: 25}, {FromFPS: 25, ToFPS: 500}} {
		if _, err := Retime(srt, SRT, f); !errors.Is(err, ErrBadFix) {
			t.Errorf("%+v: %v", f, err)
		}
	}
	if _, err := Retime("no cues", SRT, Fix{OffsetMS: 1}); !errors.Is(err, ErrNotSubtitle) {
		t.Errorf("no cues: %v", err)
	}
	if _, err := Retime(srt, SUB, Fix{OffsetMS: 1}); err == nil {
		t.Error("sub retimed")
	}
}

func TestHostileTimingIsFast(t *testing.T) {
	// Long lines of arrows and colons still take linear time.
	bad := strings.Repeat(strings.Repeat("0:", 5000)+"-->"+strings.Repeat(":0", 5000)+"\n", 100)
	start := time.Now()
	_, _ = Retime(bad, SRT, Fix{OffsetMS: 1})
	_, _ = Retime(bad, ASS, Fix{OffsetMS: 1})
	if time.Since(start) > 2*time.Second {
		t.Fatal("too slow")
	}
}

func TestClean(t *testing.T) {
	// Windows-1252 turns into UTF-8, and NULs and control characters are removed.
	b := []byte("1\n00:00:01,000 --> 00:00:02,000\nCaf\xe9 \x00na\x07ive\n")
	got, err := Clean(b, SRT)
	if err != nil || !strings.Contains(got, "Café naive") {
		t.Fatalf("%q %v", got, err)
	}
	// UTF-16 with a byte-order mark.
	u16 := []byte{0xFF, 0xFE}
	for _, r := range "1\n00:00:01,000 --> 00:00:02,000\nñ\n" {
		u16 = append(u16, byte(r), byte(r>>8))
	}
	if got, err := Clean(u16, SRT); err != nil || !strings.Contains(got, "ñ") {
		t.Fatalf("utf16: %q %v", got, err)
	}
	// A UTF-8 byte-order mark is dropped.
	if got, _ := Clean([]byte("\xEF\xBB\xBF1\n00:00:01,000 --> 00:00:02,000\nx\n"), SRT); strings.HasPrefix(got, "\uFEFF") {
		t.Fatal("bom kept")
	}
	// Not a subtitle at all.
	for _, bad := range []string{"<html><body>Login</body></html>", "just words", ""} {
		if _, err := Clean([]byte(bad), SRT); !errors.Is(err, ErrNotSubtitle) {
			t.Errorf("%q: %v", bad, err)
		}
	}
	if _, err := Clean(bytes.Repeat([]byte("a"), MaxText+1), SRT); !errors.Is(err, ErrTooBig) {
		t.Error("too big kept")
	}
}

func zipOf(t *testing.T, files map[string][]byte, method uint16) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, b := range files {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: method})
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(b)
	}
	_ = zw.Close()
	return buf.Bytes()
}

func TestUnpack(t *testing.T) {
	z := zipOf(t, map[string][]byte{"Movie.en.srt": []byte(srt), "readme.txt": []byte("hi"), "sub/Movie.ass": []byte("[Script Info]\n")}, zip.Deflate)
	es, err := Unpack(z, "x.zip")
	if err != nil || len(es) != 2 {
		t.Fatalf("%+v %v", es, err)
	}
	for _, e := range es {
		if strings.Contains(e.Name, "/") {
			t.Errorf("entry name keeps a folder: %q", e.Name)
		}
	}
	for name, bad := range map[string][]byte{
		"../evil.srt":    []byte(srt),
		"/abs.srt":       []byte(srt),
		"C:/win.srt":     []byte(srt),
		"a/../../up.srt": []byte(srt),
		"bomb.srt":       bytes.Repeat([]byte("0"), MaxEntryText+10),
	} {
		if _, err := Unpack(zipOf(t, map[string][]byte{name: bad}, zip.Deflate), "x.zip"); !errors.Is(err, ErrArchive) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// An entry with a high ratio is still a bomb even under the size cap.
	if _, err := Unpack(zipOf(t, map[string][]byte{"r.srt": bytes.Repeat([]byte("0"), 4<<20)}, zip.Deflate), "x.zip"); !errors.Is(err, ErrArchive) {
		t.Errorf("ratio: %v", err)
	}
	many := map[string][]byte{}
	for i := range MaxEntries + 1 {
		many[strings.Repeat("a", i+1)+".srt"] = []byte(srt)
	}
	if _, err := Unpack(zipOf(t, many, zip.Store), "x.zip"); !errors.Is(err, ErrArchive) {
		t.Errorf("many: %v", err)
	}
	if _, err := Unpack([]byte("Rar!\x1a\x07\x00rest"), "x.rar"); !errors.Is(err, ErrArchive) {
		t.Errorf("rar: %v", err)
	}
	if es, err := Unpack([]byte(srt), "download"); err != nil || es[0].Format != SRT {
		t.Errorf("plain: %+v %v", es, err)
	}
	if _, err := Unpack([]byte("<html>"), "download"); !errors.Is(err, ErrNotSubtitle) {
		t.Errorf("html: %v", err)
	}
	if _, err := Unpack(bytes.Repeat([]byte("a"), MaxArchive+1), "x.srt"); !errors.Is(err, ErrTooBig) {
		t.Errorf("big: %v", err)
	}
}

func FuzzRetime(f *testing.F) {
	f.Add(srt, int64(100))
	f.Add("[Events]\nDialogue: 0,0:00:01.00,0:00:02.00,x\n", int64(-100))
	f.Fuzz(func(t *testing.T, s string, off int64) {
		if len(s) > 1<<16 {
			return
		}
		_, _ = Retime(s, SRT, Fix{OffsetMS: off % MaxOffsetMS})
		_, _ = Retime(s, ASS, Fix{OffsetMS: off % MaxOffsetMS})
	})
}

func FuzzUnpackClean(f *testing.F) {
	f.Add([]byte(srt))
	f.Fuzz(func(t *testing.T, b []byte) {
		es, err := Unpack(b, "x")
		if err != nil {
			return
		}
		for _, e := range es {
			_, _ = Clean(e.Data, e.Format)
		}
	})
}

// Only a .sub that is plainly text counts as text.
func TestTextSub(t *testing.T) {
	for in, want := range map[string]bool{
		"{1}{25}Hello\n":            true,
		"\xef\xbb\xbf\r\n{1}{25}Hi": true,
		"[INFORMATION]\n":           true,
		"\x00\x00\x01\xba\x44\x00":  false,
		"":                          false,
		"Hello there\n":             false,
		"{1}{25}Hi\x00\x01 binary":  false,
	} {
		if got := TextSub([]byte(in)); got != want {
			t.Errorf("TextSub(%q) = %v", in, got)
		}
	}
}

// A zip entry that is a link is refused. The check reads the entry's file mode
// rather than raw unix bits.
func TestUnpackRefusesSymlinkEntry(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	h := &zip.FileHeader{Name: "x.srt", Method: zip.Store}
	h.SetMode(fs.ModeSymlink | 0o777)
	w, err := zw.CreateHeader(h)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("/etc/passwd"))
	_ = zw.Close()
	if _, err := Unpack(buf.Bytes(), "x.zip"); !errors.Is(err, ErrArchive) {
		t.Fatalf("symlink entry not refused: %v", err)
	}
}
