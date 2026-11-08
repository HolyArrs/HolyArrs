package frontend

import (
	"io/fs"
	"testing"
)

// The server serves Files at /. It must be rooted inside dist, not at it.
func TestFilesRootedAtDist(t *testing.T) {
	if Files == nil {
		t.Fatal("fs.Sub failed: Files is nil")
	}
	if _, err := fs.Stat(Files, ".gitkeep"); err != nil {
		if _, err2 := fs.Stat(Files, "index.html"); err2 != nil {
			t.Fatalf("Files not rooted at dist (no .gitkeep or index.html): %v", err)
		}
	}
	if _, err := fs.Stat(Files, "dist"); err == nil {
		t.Fatal("Files still contains a dist/ prefix")
	}
}
