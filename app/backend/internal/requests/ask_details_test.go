package requests

import (
	"context"
	"database/sql"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// placeholderAsk stores an ask the way an import from another app left it
// before its name was known.
func placeholderAsk(t *testing.T, r *rig, kind, ref string, ids map[string]string) int64 {
	t.Helper()
	var id int64
	err := r.st.Write(context.Background(), func(tx *sql.Tx) error {
		var err error
		id, err = database.Insert(tx, "asks", &domain.Ask{Kind: kind, TMDBID: ids["tmdb"], Name: PlaceholderName(ref), State: domain.AskAsked,
			AskedBy: r.ana.UserID, AskedAt: r.clock, Ref: ref, SourceIDs: ids, Seasons: []int{}, Albums: []domain.AskAlbum{}}, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestFillAskDetailsHealsImportedNames(t *testing.T) {
	r := showRig(t)
	ctx := context.Background()
	mv := placeholderAsk(t, r, "movie", "tmdb:693134", map[string]string{"tmdb": "693134"})
	sh := placeholderAsk(t, r, "show", "tvdb:200", map[string]string{"tvdb": "200"})
	// Not known to the details source yet: the lookup fails.
	late := placeholderAsk(t, r, "movie", "tmdb:438631", map[string]string{"tmdb": "438631"})
	n, err := r.svc.FillAskDetails(ctx)
	if err != nil || n != 2 {
		t.Fatalf("filled %d, %v", n, err)
	}
	for id, want := range map[int64]string{mv: "Dune: Part Two", sh: "Lab Show", late: "TMDB 438631"} {
		a, err := get(ctx, r.st.Read(), id)
		if err != nil || a.Name != want {
			t.Fatalf("ask %d: %q, %v; want %q", id, a.Name, err, want)
		}
		if want != "TMDB 438631" && (a.Year == nil || *a.Year == 0) {
			t.Fatalf("ask %d has no year", id)
		}
	}
	// The source answers on a later pass: the stand-in name is replaced.
	md := r.svc.Library.Metadata.(*metadata.Static)
	md.Movies = append(md.Movies, movie("Dune", 2021, "438631"))
	if n, err := r.svc.FillAskDetails(ctx); err != nil || n != 1 {
		t.Fatalf("second pass filled %d, %v", n, err)
	}
	if a, _ := get(ctx, r.st.Read(), late); a.Name != "Dune" || a.Year == nil || *a.Year != 2021 {
		t.Fatalf("late ask %+v", a)
	}
	// A request made here keeps its name.
	res, err := r.svc.Ask(ctx, r.bo, "900002")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.FillAskDetails(ctx); err != nil {
		t.Fatal(err)
	}
	if a, _ := get(ctx, r.st.Read(), res.Ask.ID); a.Name != "Paper Harbour" {
		t.Fatalf("own ask renamed to %q", a.Name)
	}
}

func TestPlaceholderName(t *testing.T) {
	if got := PlaceholderName("tvdb:81189"); got != "TVDB 81189" {
		t.Fatal(got)
	}
	if !isPlaceholder("TMDB 1396", "tvdb:81189", map[string]string{"tmdb": "1396"}) || isPlaceholder("Breaking Bad", "tvdb:81189", nil) {
		t.Fatal("placeholder check")
	}
}
