package library

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// A folder or recorded path that leaves the Library folder is never read,
// however it is written.
func TestRecordedMovieVideoStaysInRoot(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "lib")
	out := filepath.Join(base, "out")
	for _, d := range []string{filepath.Join(root, "Movie (2020)", "Disc1"), filepath.Join(out, "Disc1")} {
		if err := os.MkdirAll(d, 0o777); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{filepath.Join(root, "Movie (2020)", "Disc1", "Feature.mkv"), filepath.Join(out, "Feature.mkv"), filepath.Join(out, "Disc1", "Feature.mkv")} {
		if err := os.WriteFile(f, []byte("x"), 0o666); err != nil {
			t.Fatal(err)
		}
	}
	if v, ok := RecordedMovieVideo(root, "Movie (2020)", "Disc1/Feature.mkv"); !ok || v.Path != "Movie (2020)/Disc1/Feature.mkv" {
		t.Fatalf("inside: got %+v %v", v, ok)
	}
	bad := [][2]string{
		{"../out", "Feature.mkv"},
		{"../out", "Disc1/Feature.mkv"},
		{"Movie (2020)/../../out", "Disc1/Feature.mkv"},
		{"Movie (2020)", "../../out/Feature.mkv"},
		{"Movie (2020)", "Disc1/../../../out/Feature.mkv"},
		{"", "../out/Feature.mkv"},
		{filepath.ToSlash(out), "Feature.mkv"},
		{filepath.ToSlash(out), "Disc1/Feature.mkv"},
		{"Movie (2020)", `..\..\out\Feature.mkv`},
		{"Movie (2020)", `Disc1\..\..\..\out\Feature.mkv`},
	}
	if runtime.GOOS == "windows" {
		bad = append(bad, [2]string{"C:", "Feature.mkv"}, [2]string{`\server\share`, "Feature.mkv"}, [2]string{"//server/share", "Disc1/Feature.mkv"}, [2]string{"Movie (2020)", "NUL"})
	}
	for _, b := range bad {
		if v, ok := RecordedMovieVideo(root, b[0], b[1]); ok {
			t.Errorf("folder %q file %q: read %+v outside the Library folder", b[0], b[1], v)
		}
		if v, ok := NamedMovieVideo(root, b[0], b[1]); ok {
			t.Errorf("named: folder %q file %q: read %+v outside the Library folder", b[0], b[1], v)
		}
	}
}
