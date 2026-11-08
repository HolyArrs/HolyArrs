package discover

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

type shelfPeople struct{ fakePeople }

func (f *shelfPeople) SearchPeople(_ context.Context, query string, _ int) (metadata.PersonPage, error) {
	dept := "Acting"
	if strings.HasPrefix(query, "Writer") {
		dept = "Writing"
	}
	return metadata.PersonPage{People: []metadata.Person{{ID: "id-" + query, Name: query, Department: dept, Profile: "/p/" + query + ".jpg"}}}, nil
}

func (f *shelfPeople) TitleCrew(_ context.Context, _, _ string) (metadata.TitleCrew, error) {
	return metadata.TitleCrew{Composers: []metadata.Person{{ID: "c1", Name: "Cora Note", Profile: "/p/c.jpg"}}}, nil
}

func shelfTitles(t *testing.T, s *database.Store, n int, people []string) {
	t.Helper()
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		for i := 0; i < n; i++ {
			year := 2020
			title := mediafiles.Title{Kind: "movie", Name: fmt.Sprint("T", i), SortName: fmt.Sprint("t", i), Year: &year, SourceIDs: map[string]string{"tmdb": fmt.Sprint(500 + i)},
				MetadataSource: "tmdb", NewPartsPolicy: "all", WhyAdded: "by_hand", State: "active", ReleaseGate: "released", GateDriver: "none", Status: "released", People: people}
			if _, err := database.Insert(tx, "titles", &title, domain.System); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// route: GET /api/v1/people/shelves
func TestPeopleShelvesListsSectionsWithEnoughPeople(t *testing.T) {
	t.Parallel()
	p := &shelfPeople{fakePeople{Static: metadata.Lab()}}
	srv, s := peopleRig(t, p)
	b := string(do(t, "GET", srv.URL+"/api/v1/people/shelves", "", 200))
	if !strings.Contains(b, `"shelves":[]`) {
		t.Fatalf("empty library %s", b)
	}
	names := []string{"Writer W"}
	for i := 1; i <= 8; i++ {
		names = append(names, fmt.Sprint("Actor ", i))
	}
	srv, s = peopleRig(t, p)
	shelfTitles(t, s, 6, names)
	b = string(do(t, "GET", srv.URL+"/api/v1/people/shelves", "", 200))
	if !strings.Contains(b, `"id":"library"`) || !strings.Contains(b, `"title":"From your Library"`) || !strings.Contains(b, `"Acting":8`) || !strings.Contains(b, `"Sound":1`) || strings.Contains(b, `"id":"hot"`) {
		t.Fatalf("shelves %s", b)
	}
	srv2, _ := peopleRig(t, metadata.Lab())
	do(t, "GET", srv2.URL+"/api/v1/people/shelves", "", 503)
}

// route: GET /api/v1/people/shelves/{id}
func TestPeopleShelfCountsLibraryTitlesAndLeavesWritersOut(t *testing.T) {
	t.Parallel()
	p := &shelfPeople{fakePeople{Static: metadata.Lab()}}
	srv, s := peopleRig(t, p)
	shelfTitles(t, s, 6, []string{"Actor 1", "Writer W", "Once Only"})
	shelfTitles(t, s, 1, []string{"Actor 2", "Actor 1"})
	shelfTitles(t, s, 1, []string{"Actor 2"})
	b := string(do(t, "GET", srv.URL+"/api/v1/people/shelves/library", "", 200))
	if !strings.Contains(b, `"name":"Actor 1","department":"Acting","profile":"/p/Actor 1.jpg","note":"7 titles"`) || !strings.Contains(b, `"note":"2 titles"`) ||
		!strings.Contains(b, `"name":"Cora Note","department":"Sound"`) || strings.Contains(b, "Writer") {
		t.Fatalf("library shelf %s", b)
	}
	if strings.Index(b, "Actor 1") > strings.Index(b, "Actor 2") {
		t.Fatalf("most titles first %s", b)
	}
	b = string(do(t, "GET", srv.URL+"/api/v1/people/shelves/hot", "", 200))
	if !strings.Contains(b, `"title":"In what's hot right now"`) {
		t.Fatalf("hot shelf %s", b)
	}
	do(t, "GET", srv.URL+"/api/v1/people/shelves/nope", "", 404)
}
