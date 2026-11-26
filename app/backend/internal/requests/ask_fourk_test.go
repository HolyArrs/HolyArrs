package requests

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// A 4K ask is its own ask, approved apart from the plain one,
// and approving it adds a 4K Version on the 4K Pick rules.
func TestFourKAskAddsFourKVersion(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	plain, err := r.svc.Ask(ctx, r.ana, "693134")
	if err != nil {
		t.Fatal(err)
	}
	uhd, err := r.svc.Ask4K(ctx, r.bo, "693134")
	if err != nil || uhd.Joined || uhd.Ask.ID == plain.Ask.ID || !uhd.Ask.FourK {
		t.Fatalf("4K ask %+v %v", uhd, err)
	}
	vs, _ := r.svc.Views(ctx, "id = ?", uhd.Ask.ID)
	if len(vs) != 1 || !vs[0].FourK {
		t.Fatalf("view %+v", vs)
	}
	got, err := r.svc.Approve(ctx, plain.Ask.ID, r.luis, ApproveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// The plain approval leaves the 4K ask waiting.
	if a, _ := get(ctx, r.st.Read(), uhd.Ask.ID); a.State != domain.AskAsked {
		t.Fatalf("4K ask %+v", a)
	}
	if _, err := r.svc.Approve(ctx, uhd.Ask.ID, r.luis, ApproveOptions{}); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = r.st.Read().QueryRow(`SELECT count(*) FROM versions v JOIN pick_rules p ON p.id = v.pick_rules_id
		WHERE v.title_id = ? AND v.label = '4K' AND p.name = ?`, *got.TitleID, mediafiles.FourKName).Scan(&n)
	if n != 1 {
		t.Fatalf("4K versions %d", n)
	}
	// With a 4K Version there, a new 4K ask finds it in the Library.
	_ = r.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE asks SET state = 'available' WHERE id = ?", uhd.Ask.ID)
		return err
	})
	var in *InLibraryError
	if _, err := r.svc.Ask4K(ctx, r.ana, "693134"); !errors.As(err, &in) {
		t.Fatalf("4K again: %v", err)
	}
}

// The member who asked for the regular copy may also ask for
// 4K, and a movie already in the Library without a 4K Version takes a 4K ask.
func TestFourKAskAfterOwnAskAndInLibrary(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	plain, err := r.svc.Ask(ctx, r.ana, "693134")
	if err != nil {
		t.Fatal(err)
	}
	uhd, err := r.svc.Ask4K(ctx, r.ana, "693134")
	if err != nil || uhd.Joined || uhd.Already || uhd.Ask.ID == plain.Ask.ID || !uhd.Ask.FourK {
		t.Fatalf("own 4K ask %+v %v", uhd, err)
	}
	// Turned down, the 4K ask is gone; the plain one is approved into the Library.
	if _, err := r.svc.TurnDown(ctx, uhd.Ask.ID, r.luis, ""); err != nil {
		t.Fatal(err)
	}
	got, err := r.svc.Approve(ctx, plain.Ask.ID, r.luis, ApproveOptions{})
	if err != nil || got.TitleID == nil {
		t.Fatalf("approve %+v %v", got, err)
	}
	if has, err := HasFourK(ctx, r.st.Read(), *got.TitleID); err != nil || has {
		t.Fatalf("has 4K %v %v", has, err)
	}
	// In the Library without 4K, another member's 4K ask is made.
	again, err := r.svc.Ask4K(ctx, r.bo, "693134")
	if err != nil || !again.Ask.FourK || again.Ask.State != domain.AskAsked {
		t.Fatalf("4K on owned %+v %v", again, err)
	}
	if _, err := r.svc.Approve(ctx, again.Ask.ID, r.luis, ApproveOptions{}); err != nil {
		t.Fatal(err)
	}
	if has, err := HasFourK(ctx, r.st.Read(), *got.TitleID); err != nil || !has {
		t.Fatalf("has 4K after approve %v %v", has, err)
	}
}

// animeFolder adds a Library folder for anime of a kind and sets it in the
// Request settings.
func animeFolder(t *testing.T, r *rig, kind string) int64 {
	t.Helper()
	ctx := context.Background()
	lf, err := library.AddFolder(ctx, r.st, library.FolderInput{Path: t.TempDir(), Kind: kind, Label: "Anime"}, nil, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	set, _ := ReadSettings(ctx, r.st.Read())
	if kind == "show" {
		set.ShowAnimeLibraryFolderID = lf.ID
	} else {
		set.AnimeLibraryFolderID = lf.ID
	}
	if err := SaveSettings(ctx, r.st, set, domain.System); err != nil {
		t.Fatal(err)
	}
	return lf.ID
}

// mapCounter is an anime ID mapping that knows the listed TMDB ids and
// counts how often it was read.
type mapCounter struct {
	known map[string]bool
	calls int
}

func (m *mapCounter) read(_ context.Context, by, id string) (metadata.AnimeMap, error) {
	m.calls++
	if m.known[by+":"+id] {
		return metadata.AnimeMap{Entries: []metadata.AnimeEntry{{}}}, nil
	}
	return metadata.AnimeMap{}, metadata.ErrNotFound
}

// noMovies is a movie source that is down.
type noMovies struct{}

func (noMovies) Search(context.Context, string, int) ([]metadata.Movie, error) {
	return nil, metadata.ErrServiceUnavailable
}

func (noMovies) Movie(context.Context, string) (metadata.Movie, error) {
	return metadata.Movie{}, metadata.ErrServiceUnavailable
}

// An approved ask for an anime movie lands in the anime folder once one is
// set, an ask approved straight away and a 4K ask with no 4K folder too. A
// movie that is not anime, and anime whose approver picks a folder, go
// where they always went. With no anime folder nothing is looked up, and a
// title already in the Library never needs the metadata server for it.
func TestAnimeMovieAsksGoToTheAnimeFolder(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	md := r.svc.Metadata.(*metadata.Static)
	for _, id := range []string{"900010", "900011", "900012", "900013", "900014"} {
		m := movie("Paper Garden "+id, 2024, id)
		m.Genres, m.OriginalLanguage = []string{"Animation", "Drama"}, "ja"
		md.Movies = append(md.Movies, m)
	}
	m := &mapCounter{known: map[string]bool{}}
	r.svc.AnimeMap = m.read
	// folder asks as p, has luis approve what waits, and gives the Library
	// folder of the title's main copy.
	folder := func(p Person, id string, fourK bool, opt ApproveOptions) int64 {
		t.Helper()
		ask := r.svc.Ask
		if fourK {
			ask = r.svc.Ask4K
		}
		res, err := ask(ctx, p, id)
		if err != nil {
			t.Fatal(err)
		}
		a := res.Ask
		if a.State == domain.AskAsked {
			if a, err = r.svc.Approve(ctx, a.ID, r.luis, opt); err != nil {
				t.Fatal(err)
			}
		}
		return copies(t, r, *a.TitleID)[0].folder
	}
	plain := folder(r.ana, "900010", false, ApproveOptions{})
	if m.calls != 0 {
		t.Fatalf("the anime mapping was read %d times with no anime folder set", m.calls)
	}
	anime := animeFolder(t, r, "movie")
	if f := folder(r.ana, "900011", false, ApproveOptions{}); f != anime {
		t.Fatalf("anime movie in folder %d, want the anime folder %d", f, anime)
	}
	if f := folder(r.ana, "900002", false, ApproveOptions{}); f != plain {
		t.Fatalf("a movie that is not anime in folder %d, want %d", f, plain)
	}
	if f := folder(r.bo, "900012", false, ApproveOptions{LibraryFolderID: plain}); f != plain {
		t.Fatalf("the approver's folder %d lost to folder %d", plain, f)
	}
	if f := folder(r.ana, "900013", true, ApproveOptions{}); f != anime {
		t.Fatalf("4K anime movie with no 4K folder in folder %d, want %d", f, anime)
	}
	if f := folder(r.luis, "900014", false, ApproveOptions{}); f != anime {
		t.Fatalf("anime movie approved straight away in folder %d, want %d", f, anime)
	}
	// 900010 is in the Library: its 4K ask is approved with the server down.
	uhd, err := r.svc.Ask4K(ctx, r.bo, "900010")
	if err != nil {
		t.Fatal(err)
	}
	r.svc.Metadata = noMovies{}
	got, err := r.svc.Approve(ctx, uhd.Ask.ID, r.luis, ApproveOptions{})
	if err != nil {
		t.Fatalf("approval with the metadata server down: %v", err)
	}
	if _, ok := fourKOf(copies(t, r, *got.TitleID)); !ok {
		t.Fatalf("no 4K copy was added: %+v", copies(t, r, *got.TitleID))
	}
}
