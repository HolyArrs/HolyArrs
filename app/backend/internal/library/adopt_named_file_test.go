package library

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// The file an old app recorded becomes the movie's File even when a bigger
// video sits beside it, and a rescan keeps it as the movie's copy. A name
// that is missing, an extra or outside the folder leaves the biggest video,
// as adopting picked before.
func TestAdoptKeepsTheNamedFile(t *testing.T) {
	const dir = "Heat (1995)"
	for _, c := range []struct{ name, want string }{
		{"Heat.1995.1080p.mkv", "Heat.1995.1080p.mkv"},
		{"", "Heat.1995.2160p.mkv"},
		{"Missing.1995.mkv", "Heat.1995.2160p.mkv"},
		{"Heat (1995)-trailer.mkv", "Heat.1995.2160p.mkv"},
		{"../Heat.1995.1080p.mkv", "Heat.1995.2160p.mkv"},
		{"Extras/Heat.1995.1080p.mkv", "Heat.1995.2160p.mkv"},
	} {
		s := open(t)
		lf := addFolder(t, s)
		writeFile(t, filepath.Join(lf.Path, dir, "Heat.1995.1080p.mkv"), strings.Repeat("h", 100))
		writeFile(t, filepath.Join(lf.Path, dir, "Heat.1995.2160p.mkv"), strings.Repeat("u", 400))
		writeFile(t, filepath.Join(lf.Path, dir, "Heat (1995)-trailer.mkv"), strings.Repeat("t", 50))
		writeFile(t, filepath.Join(lf.Path, dir, "Extras", "Heat.1995.1080p.mkv"), strings.Repeat("e", 900))
		svc := &Service{Store: s, Metadata: adoptTMDB, Now: func() time.Time { return now }}
		ctx := context.Background()
		res, err := svc.Adopt(ctx, lf.ID, []AdoptChoice{{Folder: dir, TMDB: "949", File: c.name}}, domain.System)
		if err != nil || len(res) != 1 || res[0].TitleID == 0 || res[0].Error != "" {
			t.Fatalf("name %q: %+v %v", c.name, res, err)
		}
		mainFile := func() string {
			sl := slotsOf(t, s, res[0].TitleID)[0]
			if sl.FileID == nil {
				return ""
			}
			var f mediafiles.File
			if err := database.Get(ctx, s.Read(), "files", *sl.FileID, &f); err != nil {
				t.Fatal(err)
			}
			return f.RelativePath
		}
		if got := mainFile(); got != dir+"/"+c.want {
			t.Fatalf("name %q: the movie's file is %q, want %q", c.name, got, c.want)
		}
		if _, err := svc.Rescan(ctx, domain.System); err != nil {
			t.Fatal(err)
		}
		if got := mainFile(); got != dir+"/"+c.want {
			t.Fatalf("name %q: after a rescan the movie's file is %q, want %q", c.name, got, c.want)
		}
	}
}
