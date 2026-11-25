package backups

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

// A slow upload must not hold up a second restore while its bytes arrive.
func TestSlowUploadDoesNotBlockAnotherRestore(t *testing.T) {
	m := &Manager{Dir: t.TempDir()}
	pr, pw := io.Pipe()
	slow := make(chan error, 1)
	go func() { slow <- m.Stage(context.Background(), pr) }()
	_, _ = pw.Write([]byte("PK"))

	done := make(chan error, 1)
	go func() { done <- m.Stage(context.Background(), strings.NewReader("not a zip")) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a broken zip was staged")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the second restore waited for the slow upload")
	}
	_ = pw.Close()
	if err := <-slow; err == nil {
		t.Fatal("a broken slow upload was staged")
	}
}
