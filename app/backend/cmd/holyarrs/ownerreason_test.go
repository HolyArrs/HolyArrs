package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// A named volume has no host folder: the permission refusal must not send
// the person to fix the owner "on the host" only.
func TestNotWritableReasonFitsANamedVolume(t *testing.T) {
	msg := publicReason(fmt.Errorf("%w: %s", database.ErrNotWritable, "/config"))
	if strings.Contains(msg, "own the folder on the host") {
		t.Errorf("names only a host fix: %s", msg)
	}
	for _, want := range []string{":ro", "PUID/PGID", "already tried", "NFS", "user:"} {
		if !strings.Contains(msg, want) {
			t.Errorf("lacks %q: %s", want, msg)
		}
	}
	// A root start already ran the owner fix before this page shows, so a
	// plain restart repeats the same thing.
	if strings.Contains(msg, "restart is often enough") {
		t.Errorf("suggests a plain restart fixes it: %s", msg)
	}
}
