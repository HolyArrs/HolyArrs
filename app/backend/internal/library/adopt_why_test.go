package library

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

type downMovies struct{ *metadata.Static }

func (downMovies) Movie(context.Context, string) (metadata.Movie, error) {
	return metadata.Movie{}, fmt.Errorf("TMDB: %w (%w)", metadata.ErrServiceUnavailable, outbound.ErrSuppressed)
}

// The reasons a folder was not brought in are plain words: the movie by
// its name, not an id, and a server that is not answering says what to do.
func TestAdoptSaysWhyInPlainWords(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	adoptTree(t, lf.Path)
	ctx := context.Background()
	svc := &Service{Store: s, Metadata: adoptTMDB, Now: func() time.Time { return now }}
	res, err := svc.Adopt(ctx, lf.ID, []AdoptChoice{{Folder: "Dune - Part Two (2024)", TMDB: "693134"}, {Folder: "Random Stuff", TMDB: "693134"}}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if res[1].Error != "Dune: Part Two is already in the Library in another folder" {
		t.Fatalf("a movie already in the Library: %q", res[1].Error)
	}
	svc.Metadata = downMovies{adoptTMDB}
	res, _ = svc.Adopt(ctx, lf.ID, []AdoptChoice{{Folder: "Heat (1995) {tmdb-949}", TMDB: "949"}}, domain.System)
	if res[0].Error != MetadataNotAnswering || strings.Contains(res[0].Error, "TMDB") {
		t.Fatalf("the metadata server down: %q", res[0].Error)
	}
}

func (downMovies) Search(context.Context, string, int) ([]metadata.Movie, error) {
	return nil, fmt.Errorf("TMDB: %w", metadata.ErrServiceUnavailable)
}

// A scan while the metadata server is not answering says so on the page,
// and each folder says why it has no match instead of only "not matched".
func TestScanSaysTheMetadataServerIsNotAnswering(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	adoptTree(t, lf.Path)
	p, err := PlanAdoptFrom(context.Background(), s.Read(), downMovies{adoptTMDB}, lf.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if p.Warning != MetadataNotAnsweringScan {
		t.Fatalf("warning %q", p.Warning)
	}
	// Folders the server did not answer for have their own state, apart
	// from folders that truly match nothing.
	looked := 0
	for _, c := range p.Folders {
		if c.Status == "unmatched" {
			t.Fatalf("%s: unmatched while the server was down: %q", c.Folder, c.Why)
		}
		if c.Status == NotLookedUp {
			looked++
			if c.Why != MetadataNotAnswering {
				t.Fatalf("%s: %q", c.Folder, c.Why)
			}
		}
	}
	if looked == 0 || p.Counts[NotLookedUp] != looked {
		t.Fatalf("not looked up %d counts %v", looked, p.Counts)
	}
	p, _ = PlanAdoptFrom(context.Background(), s.Read(), adoptTMDB, lf.ID, "")
	if p.Warning != "" {
		t.Fatalf("a scan with the server answering warns %q", p.Warning)
	}
}

type racingMovies struct {
	*metadata.Static
	win func()
}

func (m racingMovies) Movie(ctx context.Context, id string) (metadata.Movie, error) {
	if m.win != nil {
		m.win()
	}
	return m.Static.Movie(ctx, id)
}

// Two imports of one folder at the same time: the one that comes second
// counts the folder as brought in, with its title, not as a failure.
func TestAdoptTwiceAtOnceCountsTheFolderAsBroughtIn(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	adoptTree(t, lf.Path)
	ctx := context.Background()
	first := &Service{Store: s, Metadata: adoptTMDB, Now: func() time.Time { return now }}
	pick := []AdoptChoice{{Folder: "Dune - Part Two (2024)", TMDB: "693134"}}
	var won []AdoptResult
	second := &Service{Store: s, Now: func() time.Time { return now }}
	second.Metadata = racingMovies{adoptTMDB, func() { won, _ = first.Adopt(ctx, lf.ID, pick, domain.System) }}
	res, err := second.Adopt(ctx, lf.ID, pick, domain.System)
	if err != nil || len(won) != 1 || won[0].TitleID == 0 {
		t.Fatalf("the first import: %v %+v", err, won)
	}
	if res[0].Error != "" || res[0].TitleID != won[0].TitleID {
		t.Fatalf("the second import of the same folder: %+v, want title %d and no reason", res[0], won[0].TitleID)
	}
}
