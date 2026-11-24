package files

import (
	"strings"
	"testing"
)

func TestShareFixPerKind(t *testing.T) {
	cases := []struct {
		kind   string
		has    []string
		hasNot []string
	}{
		{"9p", []string{"Docker Desktop", "named volume", "holyarrs-config:/config", "Media folders can stay"}, []string{"local storage", "cache"}},
		{"fakeowner", []string{"Docker Desktop", "named volume", "holyarrs-config:/config", "HOLYARRS_ALLOW_DESKTOP_FOLDER=1"}, []string{"cache"}},
		{"fuse", []string{"/mnt/<pool>/appdata", "/mnt/disk1/appdata/holyarrs", "named volume", "HOLYARRS_ALLOW_FUSE_FOLDER=1", "warns on every start"}, []string{"cache"}},
		{"nfs", []string{"local disk"}, nil},
		{"smb", []string{"local disk"}, nil},
		{"cifs", []string{"local disk"}, nil},
		{"some-new-kind", []string{"local disk"}, nil},
	}
	for _, c := range cases {
		t.Run(c.kind, func(t *testing.T) {
			fix := ShareFix(c.kind)
			for _, w := range c.has {
				if !strings.Contains(fix, w) {
					t.Errorf("ShareFix(%q) = %q, lacks %q", c.kind, fix, w)
				}
			}
			for _, w := range c.hasNot {
				if strings.Contains(strings.ToLower(fix), strings.ToLower(w)) {
					t.Errorf("ShareFix(%q) = %q, should not say %q", c.kind, fix, w)
				}
			}
		})
	}
}

func TestUnraidUserShareOnlyShfs(t *testing.T) {
	cases := []struct {
		kind, fstype string
		want         bool
	}{
		{"fuse", "fuse.shfs", true},
		{"fuse", "fuse.mergerfs", false},
		{"fuse", "fuse.rclone", false},
		{"fuse", "", false},
		{"nfs", "fuse.shfs", false},
		{"", "fuse.shfs", false},
	}
	for _, c := range cases {
		if got := UnraidUserShare(c.kind, c.fstype); got != c.want {
			t.Errorf("UnraidUserShare(%q, %q) = %v", c.kind, c.fstype, got)
		}
	}
}
