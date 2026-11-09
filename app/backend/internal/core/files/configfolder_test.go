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
		{"fakeowner", []string{"Docker Desktop", "named volume", "holyarrs-config:/config"}, []string{"cache"}},
		{"fuse", []string{"/mnt/<pool>/appdata", "named volume"}, []string{"cache"}},
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
