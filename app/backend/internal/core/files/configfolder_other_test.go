//go:build !linux

package files

import "testing"

func TestMountTypeOffLinux(t *testing.T) {
	if got := MountType("."); got != "" {
		t.Fatalf("want empty off Linux, got %q", got)
	}
}
