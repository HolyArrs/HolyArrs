package store

import (
	"errors"
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
