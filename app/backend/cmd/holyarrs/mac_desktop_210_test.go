package main

import (
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/files"
)

// On Docker Desktop for Mac a bind mount of /Users/... or /Volumes/...
// is seen inside the container as one of these statfs kinds. The page that
// refuses to start must name the override that lifts exactly that kind,
// or the person has no way past it short of moving /config.
func TestMacDesktopRefusalNamesItsOverride(t *testing.T) {
	cases := []struct {
		name, kind, override string
	}{
		{"fakeowner (Docker Desktop 4.74+, virtiofs share)", "fakeowner", files.EnvAllowDesktopFolder},
		{"fuse.grpcfuse (older gRPC FUSE sharing)", "fuse", files.EnvAllowFuseFolder},
		{"osxfs (legacy sharing, FUSE magic)", "fuse", files.EnvAllowFuseFolder},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			msg := publicReason(shareErr(c.kind))
			if !strings.Contains(msg, c.override+"=1") {
				t.Errorf("page does not name %s=1:\n%s", c.override, msg)
			}
		})
	}
}

// The exact text Vartkat saw must not come back unchanged.
func TestVartkatMessageIsNotTheWholeStory(t *testing.T) {
	seen := "the config folder is on a shared or network folder (fakeowner). This folder is shared from the Mac by Docker Desktop, where file locks do not work, so the database is not safe there. Put /config on a named volume (holyarrs-config:/config). Media folders can stay where they are."
	if got := publicReason(shareErr("fakeowner")); got == seen {
		t.Errorf("page still shows the #210 text with no way past it:\n%s", got)
	}
}
