package database

import (
	"errors"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/files"
)

// Every variant: each share kind a Mac (or other host) can hand the
// container. Without its override the refusal must name a way out the
// person can act on; with the override set, the folder must be used.
func TestShareRefusalVariants210(t *testing.T) {
	cases := []struct {
		kind, mount, override string
	}{
		{"fakeowner", "fakeowner", files.EnvAllowDesktopFolder}, // Docker Desktop 4.74+ (virtiofs)
		{"fuse", "fuse.grpcfuse", files.EnvAllowFuseFolder},     // older gRPC FUSE sharing
		{"fuse", "fuse.osxfs", files.EnvAllowFuseFolder},        // legacy osxfs
		{"fuse", "fuse.mergerfs", files.EnvAllowFuseFolder},
		{"9p", "9p", ""},   // Docker Desktop on Windows
		{"nfs", "nfs", ""}, // network shares: no override, local disk only
		{"cifs", "cifs", ""},
		{"smb2", "smb3", ""},
	}
	for _, c := range cases {
		t.Run(c.mount, func(t *testing.T) {
			check := func(string) (bool, string, error) { return true, c.kind, nil }
			mount := func(string) string { return c.mount }
			t.Setenv(files.EnvAllowDesktopFolder, "")
			t.Setenv(files.EnvAllowFuseFolder, "")
			_, err := checkFolder(t.TempDir(), check, mount, false)
			var ns *NetShareError
			if !errors.As(err, &ns) || ns.Kind != c.kind {
				t.Fatalf("want a %s refusal, got %v", c.kind, err)
			}
			if !strings.Contains(err.Error(), "named volume") && !strings.Contains(err.Error(), "local disk") {
				t.Errorf("refusal names no place to move /config:\n%v", err)
			}
			if c.override == "" {
				return
			}
			if !strings.Contains(files.ShareFix(c.kind), c.override+"=1") {
				t.Errorf("page text does not name %s=1:\n%s", c.override, files.ShareFix(c.kind))
			}
			t.Setenv(c.override, "1")
			if _, err := checkFolder(t.TempDir(), check, mount, false); err != nil {
				t.Errorf("%s=1 set, folder still refused: %v", c.override, err)
			}
		})
	}
}
