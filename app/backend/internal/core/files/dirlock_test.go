package files

import (
	"errors"
	"fmt"
	"os"
	"testing"
)

func TestLockDirIsExclusiveAndReleased(t *testing.T) {
	dir := t.TempDir()
	unlock, err := LockDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LockDir(dir); !errors.Is(err, ErrDirLocked) {
		t.Fatalf("second lock: %v, want ErrDirLocked", err)
	}
	unlock()
	unlock() // twice is harmless
	again, err := LockDir(dir)
	if err != nil {
		t.Fatalf("lock after release: %v", err)
	}
	again()
}

func TestLockDirGoesOnWhenLocksAreUnsupported(t *testing.T) {
	orig := lockFile
	t.Cleanup(func() { lockFile = orig })
	lockFile = func(*os.File) error { return fmt.Errorf("%w: ENOLCK", errLockUnsupported) }
	unlock, err := LockDir(t.TempDir())
	if err != nil {
		t.Fatalf("unsupported locks refused: %v", err)
	}
	unlock()
	lockFile = func(*os.File) error { return ErrDirLocked }
	if _, err := LockDir(t.TempDir()); !errors.Is(err, ErrDirLocked) {
		t.Fatalf("held lock: %v, want ErrDirLocked", err)
	}
}
