package mediafiles

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// The fixtures are real files made by ffmpeg, a few seconds long.
var wholeFiles = []string{"intact.mkv", "intact.webm", "intact.mp4", "intact-faststart.mp4", "intact-fragmented.mp4"}

func TestTruncatedLetsWholeFilesThrough(t *testing.T) {
	for _, name := range wholeFiles {
		got, err := Truncated(filepath.Join("testdata", name))
		if err != nil || got != "" {
			t.Errorf("%s: %q, %v", name, got, err)
		}
	}
}

func TestTruncatedSeesACutFile(t *testing.T) {
	for _, name := range wholeFiles {
		b, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		for _, cut := range []int{len(b) / 2, len(b) - 1, len(b) * 9 / 10, 1000, 100} {
			got := TruncatedReaderAt(bytes.NewReader(b[:cut]), int64(cut))
			if got == "" {
				t.Errorf("%s cut at %d of %d was let through", name, cut, len(b))
			}
		}
	}
}

func TestTruncatedMP4WithoutItsMovieBox(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "intact.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	i := bytes.Index(b, []byte("moov"))
	if i < 4 {
		t.Fatal("the fixture has no moov")
	}
	// Cut cleanly at the start of the movie box: every box that is left is whole.
	if got := TruncatedReaderAt(bytes.NewReader(b[:i-4]), int64(i-4)); got != NoIndex {
		t.Fatalf("got %q", got)
	}
}

func TestTruncatedLeavesOtherThingsAlone(t *testing.T) {
	for name, b := range map[string][]byte{"empty": nil, "text": []byte("hello world, this is not a video"), "short": {0x1a, 0x45, 0xdf, 0xa3},
		"avi": append([]byte("RIFF\x00\x10\x00\x00AVI LIST"), make([]byte, 64)...)} {
		if got := TruncatedReaderAt(bytes.NewReader(b), int64(len(b))); got != "" {
			t.Errorf("%s: %q", name, got)
		}
	}
}

func TestTruncatedHostileHeadersDoNotHang(t *testing.T) {
	huge := []byte{0, 0, 0, 1, 'm', 'd', 'a', 't', 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	if got := TruncatedReaderAt(bytes.NewReader(huge), int64(len(huge))); got != "" {
		t.Errorf("absurd box size: %q", got)
	}
	loop := bytes.Repeat([]byte{0, 0, 0, 8, 'f', 'r', 'e', 'e'}, 20000)
	copy(loop[4:], "ftyp")
	_ = TruncatedReaderAt(bytes.NewReader(loop), int64(len(loop)))
}

func TestTruncatedMP4WholeFileWithTrailingBytes(t *testing.T) {
	for _, name := range []string{"intact.mp4", "intact-faststart.mp4", "intact-fragmented.mp4"} {
		b, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		for _, tail := range [][]byte{bytes.Repeat([]byte{0xff}, 16), bytes.Repeat([]byte("trailing text. "), 3)} {
			whole := append(append([]byte{}, b...), tail...)
			if got := TruncatedReaderAt(bytes.NewReader(whole), int64(len(whole))); got != "" {
				t.Errorf("%s with %d stray bytes: %q", name, len(tail), got)
			}
		}
	}
}
