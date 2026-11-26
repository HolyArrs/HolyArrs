package updates_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/updates"
)

// The update notice never depends on the signed list: with the list
// unreachable, the release feed still reports the new version.
func TestNoticeShowsWhenListIsDown(t *testing.T) {
	w, _, srv, pages := listWorld(t)
	srv.down, pages.down = true, true
	f := newFeed(t)
	f.body = `{"tag_name": "v1.2.0", "html_url": "https://github.com/HolyArrs/HolyArrs/releases/tag/v1.2.0"}`
	w.e.ReleaseFeed = f.srv.URL
	if i := w.e.CheckUpdate(context.Background()); !i.Newer || i.Latest != "v1.2.0" || i.Problem != "" {
		t.Fatalf("notice with the list down: %+v", i)
	}
}

// A fresh install shows the notice and installs nothing by itself.
func TestDefaultsNoticeOnInstallOff(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	if i := w.e.Update(ctx); !i.On {
		t.Fatalf("the update notice is off by default: %+v", i)
	}
	t.Setenv(updates.EnvMode, "")
	if m, locked := w.e.Mode(ctx); m != updates.ModeOff || locked {
		t.Fatalf("default mode %q locked %v", m, locked)
	}
}

// The pinned key is the public half of the release key; the file and the
// code always match (no file: nothing pinned).
func TestPinnedKeyMatchesReleaseKeyFile(t *testing.T) {
	b, err := os.ReadFile("../../../../deploy/release/updates/keys/main.pub")
	if os.IsNotExist(err) {
		if len(updates.PinnedKeys) != 0 {
			t.Fatal("a key is pinned but deploy/release/updates/keys/main.pub is missing")
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	k, err := updates.ParsePublicKey(string(b))
	if err != nil {
		t.Fatal(err)
	}
	if len(updates.PinnedKeys) == 0 || updates.PinnedKeys[0] != k.Line() || !strings.Contains(string(b), k.Line()) {
		t.Fatalf("PinnedKeys[0] must be %s", k.Line())
	}
}
