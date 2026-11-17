package library

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Leftovers test 8: two videos in a movie's folder stay notes with
// movie_more_than_one; a match by hand links the chosen one, and replace
// turns it back into a note when the other is picked.
func TestMovieLeftoversMatchByHand(t *testing.T) {
	svc, s, id, rel := movieWithFoundCopy(t, "")
	ctx := context.Background()
	other := filepath.ToSlash(filepath.Join(filepath.Dir(rel), "Dune.Part.Two.2024.2160p.WEB-DL.mkv"))
	var lf mediafiles.LibraryFolder
	v := versionsOf(t, s, id)[0]
	if err := database.Get(ctx, s.Read(), "library_folders", v.LibraryFolderID, &lf); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(other)), "second copy")
	if _, err := svc.Rescan(ctx, domain.System); err != nil {
		t.Fatal(err)
	}
	ls, err := svc.Leftovers(ctx, id)
	if err != nil || len(ls) != 2 {
		t.Fatalf("leftovers %+v %v", ls, err)
	}
	by := map[string]LeftoverView{}
	for _, l := range ls {
		if l.Reason != "movie_more_than_one" {
			t.Errorf("reason %+v", l)
		}
		by[l.RelativePath] = l
	}
	if sl := slotsOf(t, s, id)[0]; sl.FileID != nil {
		t.Fatal("a copy was linked with two to choose from")
	}
	out, err := svc.MatchLeftovers(ctx, id, []LeftoverMatch{{FileID: by[rel].FileID}}, false, domain.System)
	if err != nil || !out[0].Linked {
		t.Fatalf("match %+v %v", out, err)
	}
	if sl := slotsOf(t, s, id)[0]; sl.FileID == nil || *sl.FileID != by[rel].FileID {
		t.Fatalf("slot %+v", sl)
	}
	if out, _ := svc.MatchLeftovers(ctx, id, []LeftoverMatch{{FileID: by[other].FileID}}, false, domain.System); out[0].Refused != "episode_has_file" {
		t.Fatalf("second copy without replace %+v", out)
	}
	if out, _ := svc.MatchLeftovers(ctx, id, []LeftoverMatch{{FileID: by[other].FileID}}, true, domain.System); !out[0].Linked {
		t.Fatalf("replace %+v", out)
	}
	ls, _ = svc.Leftovers(ctx, id)
	if len(ls) != 1 || ls[0].RelativePath != rel || ls[0].Reason != "replaced_by_hand" {
		t.Fatalf("after replace %+v", ls)
	}
}
