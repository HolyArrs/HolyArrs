package health

import (
	"context"
	"strings"
	"testing"
)

// The disk that holds /config: a Warning under 1 GB free, an Error under
// 256 MB, nothing when free space is unknown or no folder is set.
func TestConfigDiskSpace(t *testing.T) {
	w := newWorld(t)
	find := func() *Result {
		rs, err := w.e.Check(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for i := range rs {
			if rs[i].Kind == "config_disk" {
				return &rs[i]
			}
		}
		return nil
	}
	if r := find(); r != nil {
		t.Fatalf("no ConfigDir: no check, got %+v", r)
	}
	w.e.ConfigDir = t.TempDir()
	for _, c := range []struct {
		free int64
		want string
	}{{50 << 30, OK}, {900 << 20, Warning}, {100 << 20, Error}} {
		w.free = c.free
		r := find()
		if r == nil || r.State != c.want {
			t.Fatalf("free %d: want %s, got %+v", c.free, c.want, r)
		}
		if c.want != OK && (!strings.Contains(r.Message, "config folder") || !strings.Contains(r.Message, "MB")) {
			t.Errorf("free %d: message %q", c.free, r.Message)
		}
	}
	w.free = -1
	if r := find(); r != nil && r.State != OK {
		t.Fatalf("unknown free space must not warn: %+v", r)
	}
}
