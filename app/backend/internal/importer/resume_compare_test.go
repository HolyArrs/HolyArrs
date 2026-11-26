package importer

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// On a resumed import, a target of the right size whose first and last MiB
// match the source but whose middle differs is another file, not the copy
// this import made, and is refused.
func TestResumeRefusesTargetThatSharesOnlyItsEnds(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "in", "a.mkv"), filepath.Join(dir, "lib", "a.mkv")
	body := make([]byte, 3<<20)
	other := make([]byte, len(body))
	copy(other, body)
	other[len(other)/2] = 1
	for p, b := range map[string][]byte{src: body, dst: other} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	r := place(mediafiles.PlanStep{SourceFile: src, TargetPath: dst, Method: "hardlink"}, Ops{Link: os.Link, Copy: copyFile, Move: renameNoReplace}, time.Now().Add(-time.Minute))
	if r.Outcome == "placed" {
		t.Fatalf("a different file was taken as the resumed copy: %+v", r)
	}
	// The same bytes in full are this import's finished copy.
	if err := os.WriteFile(dst, body, 0o644); err != nil {
		t.Fatal(err)
	}
	if r := place(mediafiles.PlanStep{SourceFile: src, TargetPath: dst, Method: "hardlink"}, Ops{Link: os.Link, Copy: copyFile, Move: renameNoReplace}, time.Now().Add(-time.Minute)); r.Outcome != "placed" {
		t.Fatalf("the finished copy was refused: %+v", r)
	}
}
