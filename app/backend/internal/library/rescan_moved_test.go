package library

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// when every known file of a small Library (or of one title) is
// renamed or moved inside its own title folder, one rescan marks the old
// paths missing. The new videos show the drive is mounted, so the
// dropped-mount guard must not hold the change back until a second rescan.
// Files gone with nothing new beside them still trip the guard.
func TestRescanMovedInTitleFolderMarksMissingFirstPass(t *testing.T) {
	for _, tc := range []struct {
		name    string
		replace bool
	}{{"moved", true}, {"deleted", false}} {
		t.Run(tc.name, func(t *testing.T) {
			s := open(t)
			lf := addFolder(t, s)
			adoptTree(t, lf.Path)
			svc := &Service{Store: s, Metadata: adoptTMDB, Now: func() time.Time { return now }}
			ctx := context.Background()
			if res, err := svc.Adopt(ctx, lf.ID, []AdoptChoice{{Folder: "Dune - Part Two (2024)", TMDB: "693134"}, {Folder: "Heat (1995) {tmdb-949}", TMDB: "949"}}, domain.System); err != nil || res[0].TitleID == 0 {
				t.Fatalf("%+v %v", res, err)
			}
			known, err := database.Select[mediafiles.File](ctx, s.Read(), "files", "state = 'present'")
			if err != nil || len(known) < 2 {
				t.Fatalf("known %d %v", len(known), err)
			}
			for _, f := range known {
				old := filepath.Join(lf.Path, filepath.FromSlash(f.RelativePath))
				if !tc.replace {
					if err := os.Remove(old); err != nil {
						t.Fatal(err)
					}
					continue
				}
				// Same title folder, new name in a new subfolder.
				top, _, _ := strings.Cut(f.RelativePath, "/")
				to := filepath.Join(lf.Path, top, "moved", "renamed-"+path.Base(f.RelativePath))
				if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(old, to); err != nil {
					t.Fatal(err)
				}
			}
			r, err := svc.Rescan(ctx, domain.System)
			if err != nil {
				t.Fatal(err)
			}
			var missing int
			_ = s.Read().QueryRow("SELECT count(*) FROM files WHERE state = 'missing'").Scan(&missing)
			if tc.replace {
				if r.Unreachable != 0 || r.Missing != len(known) || missing != len(known) {
					t.Fatalf("one rescan: %+v, %d of %d missing", r, missing, len(known))
				}
				return
			}
			if r.Unreachable != 1 || r.Missing != 0 || missing != 0 {
				t.Fatalf("guard: %+v, %d missing", r, missing)
			}
		})
	}
}
