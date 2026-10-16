package jobs

import (
	"context"
	"strings"
	"testing"
)

// A Job that panics fails with the panic's type only, never its text.
func TestJobPanicKeepsItsText(t *testing.T) {
	err := safeCall(context.Background(), func(context.Context, *Run) error { panic("private detail q7x") }, &Run{})
	if err == nil || strings.Contains(err.Error(), "q7x") || !strings.Contains(err.Error(), "string") {
		t.Fatalf("err %v", err)
	}
}
