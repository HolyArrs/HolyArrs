package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// TestAdoptMovieCopy: an existing video in a movie Version's own folder
// becomes that Version's File; a Version of another title, a path outside
// the Version's folder, a non-video and a second copy are all refused.
func TestAdoptMovieCopy(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	lf := addFolder(t, s)
	svc := &Service{Store: s, Now: func() time.Time { return now }}
	a, err := AddMovie(ctx, s, dune, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	b, err := AddMovie(ctx, s, future, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	vA, vB := versionsOf(t, s, a)[0], versionsOf(t, s, b)[0]
	write := func(rel, body string) string {
		p := filepath.Join(lf.Path, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return rel
	}
	good := write(vA.FolderName+"/Dune.Part.Two.2024.1080p.WEB-DL.H264-GRP.mkv", "video bytes")
	other := write(vB.FolderName+"/The.Lantern.Keepers.2027.mkv", "other video")
	notVideo := write(vA.FolderName+"/notes.txt", "text")

	// Refusals first: nothing may change on any of them.
	for name, c := range map[string]struct {
		title, version int64
		in             CopyAdoption
		want           error
	}{
		"version of another title": {a, vB.ID, CopyAdoption{RelativePath: good}, database.ErrNotFound},
		"title that does not own":  {b, vA.ID, CopyAdoption{RelativePath: other}, database.ErrNotFound},
		"outside the version":      {a, vA.ID, CopyAdoption{RelativePath: other}, ErrCopyChoice},
		"parent escape":            {a, vA.ID, CopyAdoption{RelativePath: "../" + filepath.Base(lf.Path) + "/" + good}, ErrCopyChoice},
		"not a video":              {a, vA.ID, CopyAdoption{RelativePath: notVideo}, ErrCopyChoice},
		"missing file":             {a, vA.ID, CopyAdoption{RelativePath: vA.FolderName + "/nope.mkv"}, ErrCopyChoice},
	} {
		if _, err := svc.AdoptMovieCopy(ctx, c.title, c.version, c.in, domain.System); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", name, err, c.want)
		}
	}
	if n := countRows(t, s, "files"); n != 0 {
		t.Fatalf("a refused adoption left %d File rows", n)
	}

	f, err := svc.AdoptMovieCopy(ctx, a, vA.ID, CopyAdoption{RelativePath: good}, domain.System)
	if err != nil {
		t.Fatalf("adopt: %v", err)
	}
	if f.ID == 0 || f.VersionID != vA.ID || f.Origin != "adopted" || f.PlacedBy != "none" || f.RelativePath != good || f.State != "present" {
		t.Fatalf("file %+v", f)
	}
	sl := slotsOf(t, s, a)
	if len(sl) != 1 || sl[0].FileID == nil || *sl[0].FileID != f.ID || sl[0].State != mediafiles.SlotInLibrary {
		t.Fatalf("slot %+v", sl)
	}
	if sb := slotsOf(t, s, b); sb[0].FileID != nil {
		t.Fatalf("the other movie's slot changed %+v", sb)
	}
	if _, err := os.Stat(filepath.Join(lf.Path, filepath.FromSlash(good))); err != nil {
		t.Fatalf("adoption moved the file: %v", err)
	}
	// A second copy for the same Version is refused.
	second := write(vA.FolderName+"/Dune.Part.Two.2024.2160p.mkv", "bigger")
	if _, err := svc.AdoptMovieCopy(ctx, a, vA.ID, CopyAdoption{RelativePath: second}, domain.System); !errors.Is(err, ErrCopyChoice) {
		t.Fatalf("second copy: %v", err)
	}
}

func countRows(t *testing.T, s *database.Store, table string) int {
	t.Helper()
	var n int
	if err := s.Read().QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
