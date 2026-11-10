package library

import (
	"os"
	"path/filepath"
	"testing"
)

// Moving a linked file re-points the link and never reads its target.
func TestMoveFileKeepsLinkAndNeverReadsTarget(t *testing.T) {
	mount, lib := t.TempDir(), t.TempDir()
	target := filepath.Join(mount, "real.mkv")
	if err := os.WriteFile(target, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	from := filepath.Join(lib, "Old (2020)", "old.mkv")
	to := filepath.Join(lib, "New (2020)", "New (2020).mkv")
	if err := os.MkdirAll(filepath.Dir(from), 0o755); err != nil {
		t.Fatal(err)
	}
	// A relative link, so the move must keep it pointing at the same file.
	rel, _ := filepath.Rel(filepath.Dir(from), target)
	if err := os.Symlink(rel, from); err != nil {
		t.Skip("symlinks not available:", err)
	}
	// An unreadable target proves nothing opens it.
	if err := os.Chmod(target, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(target, 0o644)

	proof, err := prepareFileProof(from)
	if err != nil || !proof.link || !proofSourceUnchanged(from, proof) {
		t.Fatalf("link proof: %+v %v", proof, err)
	}
	if err := moveRecordedFile(from, to, proof.value); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(from); !os.IsNotExist(err) {
		t.Fatalf("old link still there: %v", err)
	}
	got, _, ok := linkTarget(to)
	if !ok || got != target {
		t.Fatalf("new link points to %q (link %v), want %q", got, ok, target)
	}
	// A second try after a cut-off move is clean, and nothing is replaced.
	if err := os.Symlink(target, from); err != nil {
		t.Fatal(err)
	}
	if err := moveFile(from, to); err != nil {
		t.Fatalf("finishing a cut-off link move: %v", err)
	}
	other := filepath.Join(mount, "other.mkv")
	_ = os.WriteFile(other, []byte("x"), 0o644)
	_ = os.Symlink(other, from)
	if err := moveFile(from, to); err == nil {
		t.Fatal("a different link at the target was replaced")
	}
}
