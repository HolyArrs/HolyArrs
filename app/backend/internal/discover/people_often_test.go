package discover

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// echoPeople finds every searched name as itself and has 16 TV credits.
type echoPeople struct{ fakePeople }

func (e *echoPeople) SearchPeople(_ context.Context, q string, page int) (metadata.PersonPage, error) {
	return metadata.PersonPage{People: []metadata.Person{{ID: "9" + q[:1], Name: q, Department: "Acting", KnownFor: []metadata.Movie{}}}, Page: page}, nil
}

func (e *echoPeople) PersonShowCount(context.Context, string) (int, error) { return 16, nil }

func TestOftenWorksWithCountsLibraryShowsAndHidesUnderFour(t *testing.T) {
	t.Parallel()
	p := &echoPeople{fakePeople{Static: metadata.Lab()}}
	srv, s := peopleRig(t, p)
	add := func(kind, name, tmdb string, people ...string) {
		t.Helper()
		year := 2020
		err := s.Write(context.Background(), func(tx *sql.Tx) error {
			title := mediafiles.Title{Kind: kind, Name: name, SortName: name, Year: &year, SourceIDs: map[string]string{"tmdb": tmdb}, MetadataSource: "tmdb", NewPartsPolicy: "all",
				WhyAdded: "by_hand", State: "active", ReleaseGate: "released", GateDriver: "none", Status: "released", People: people}
			_, err := database.Insert(tx, "titles", &title, domain.System)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	type page struct {
		OftenWith []metadata.Person `json:"often_with"`
		Stats     PersonStats       `json:"stats"`
	}
	get := func() page {
		var x page
		if err := json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/people/11", "", 200), &x); err != nil {
			t.Fatal(err)
		}
		return x
	}
	// A show names Alex Kim in its stored cast; a movie credit (101) counts by id.
	add("show", "Harbor", "501", "Alex Kim", "Bea Ray", "Cy Dunn", "Bea Ray")
	add("movie", "Alpha", "101", "Bea Ray", "Dee Fox")
	add("movie", "Elsewhere", "777", "Bea Ray", "Eli Gray", "Fay Holt", "Gus Ide")
	x := get()
	if len(x.OftenWith) != 0 {
		t.Fatalf("three people should stay hidden: %+v", x.OftenWith)
	}
	if x.Stats.Films != 3 || x.Stats.Shows != 16 || x.Stats.Titles != 19 {
		t.Fatalf("stats %+v", x.Stats)
	}
	add("show", "Ice Road", "502", "Alex Kim", "Bea Ray", "Hal Jett", "Cy Dunn")
	x = get()
	if len(x.OftenWith) != 4 || x.OftenWith[0].Name != "Bea Ray" || x.OftenWith[1].Name != "Cy Dunn" {
		t.Fatalf("often with %+v", x.OftenWith)
	}
}
