package parts

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

const webRoutes = "../../../../frontend/src/routes"

// Every page of the web app (each +page.svelte) and every menu entry of
// the app frame belongs to a part or is marked core in Screens; a row for a
// page that is gone fails too.
func TestEveryScreenHasAPart(t *testing.T) {
	pages := map[string]bool{}
	err := filepath.WalkDir(webRoutes, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "+page.svelte" {
			return err
		}
		rel, _ := filepath.Rel(webRoutes, filepath.Dir(p))
		// Route groups like (home) do not show in the address.
		var segs []string
		for _, seg := range strings.Split(filepath.ToSlash(rel), "/") {
			if seg != "." && !(strings.HasPrefix(seg, "(") && strings.HasSuffix(seg, ")")) {
				segs = append(segs, seg)
			}
		}
		path := "/" + strings.Join(segs, "/")
		if path == "" {
			path = "/"
		}
		pages[path] = true
		if _, ok := Screens()[path]; !ok {
			t.Errorf("page %s belongs to no part and is not marked core in internal/core/parts/table.go", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) < 30 {
		t.Fatalf("found only %d pages", len(pages))
	}
	for s := range Screens() {
		if !pages[s] {
			t.Errorf("Screens lists %s, but there is no such page", s)
		}
	}
	// The menu lives in the layout and in (frame)/nav.ts (the sidebar, tab bar and palette).
	var menu []byte
	for _, f := range []string{filepath.Join(webRoutes, "+layout.svelte"), filepath.Join(webRoutes, "(frame)", "nav.ts")} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		menu = append(menu, b...)
	}
	hrefs := regexp.MustCompile(`href: '([^'?]+)[^']*'`).FindAllStringSubmatch(string(menu), -1)
	if len(hrefs) < 6 {
		t.Fatalf("found only %d menu entries", len(hrefs))
	}
	for _, m := range hrefs {
		if _, ok := ScreenOwner(m[1]); !ok {
			t.Errorf("menu entry %s belongs to no part", m[1])
		}
	}
}

// The YouTube part is registered but held by the youtube feature switch:
// with the switch off it is in no list, is off, and refuses to be switched;
// its Job kinds and worker stage belong to it or the core.
func TestYouTubePartIsHeldByItsFlag(t *testing.T) {
	if !Valid(YouTube) || YouTube.Label() != "YouTube" {
		t.Fatalf("YouTube part: valid %v, label %q", Valid(YouTube), YouTube.Label())
	}
	for _, k := range []string{"youtube_check", "youtube_download", "youtube_adopt"} {
		if JobOwner(k) != Of(YouTube) {
			t.Errorf("%s belongs to %q", k, JobOwner(k))
		}
	}
	if LoopOwner("jobs_youtube") != Core {
		t.Errorf("jobs_youtube belongs to %q", LoopOwner("jobs_youtube"))
	}
	t.Setenv("HOLYARRS_FLAGS", "")
	if slices.Contains(All(), YouTube) || !HeldOff(Of(YouTube)) {
		t.Fatalf("flag off: All() = %v", All())
	}
	c := &Control{}
	if c.On(YouTube) || !c.KindOff("youtube_download") || slices.Contains(c.Off(), YouTube) {
		t.Fatalf("flag off: on %v, off %v", c.On(YouTube), c.Off())
	}
	var held HeldError
	if err := c.Set(context.Background(), YouTube, true, domain.System); !errors.As(err, &held) || err.Error() != "YouTube is not available; turn on the youtube feature first" {
		t.Fatalf("flag off: Set = %v", err)
	}
	if _, err := c.Restart(context.Background(), YouTube); !errors.As(err, &held) {
		t.Fatalf("flag off: Restart = %v", err)
	}
	t.Setenv("HOLYARRS_FLAGS", "youtube")
	if !slices.Contains(All(), YouTube) || HeldOff(Of(YouTube)) || !c.On(YouTube) || c.KindOff("youtube_download") {
		t.Fatalf("flag on: All() = %v, on %v", All(), c.On(YouTube))
	}
}
