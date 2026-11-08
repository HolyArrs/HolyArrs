package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
)

func TestHomeRowsOverHTTP(t *testing.T) {
	t.Parallel()
	srv, s, _ := setup(t)
	type answer struct {
		Rows       []string            `json:"rows"`
		Owner      bool                `json:"owner"`
		Arrived    []discover.HomeCard `json:"arrived"`
		NewSeasons []discover.HomeCard `json:"new_seasons"`
	}
	get := func() answer {
		var a answer
		body := do(t, "GET", srv.URL+"/api/v1/home", "", 200)
		if err := json.Unmarshal(body, &a); err != nil {
			t.Fatal(err)
		}
		return a
	}
	if a := get(); !slices.Equal(a.Rows, []string{"welcome"}) {
		t.Fatalf("empty household: %v", a.Rows)
	}
	now := time.Now()
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		ins := func(table string, v any) error { _, err := database.Insert(tx, table, v, domain.System); return err }
		movie := mediafiles.Title{Kind: "movie", Name: "Fresh Movie", SortName: "fresh movie", MetadataSource: "tmdb", WhyAdded: "by_hand", State: "active", NewPartsPolicy: "all", ReleaseGate: "released", GateDriver: "none", Status: "released"}
		wanted := mediafiles.Title{Kind: "movie", Name: "Wanted Movie", SortName: "wanted movie", MetadataSource: "tmdb", WhyAdded: "by_hand", State: "active", NewPartsPolicy: "all", ReleaseGate: "released", GateDriver: "none", Status: "released"}
		for _, title := range []*mediafiles.Title{&movie, &wanted} {
			if err := ins("titles", title); err != nil {
				return err
			}
			item := mediafiles.Item{TitleID: title.ID, Kind: "movie", Name: &title.Name}
			if err := ins("items", &item); err != nil {
				return err
			}
			version := mediafiles.Version{TitleID: title.ID, Label: "Main", IsMain: true, LibraryFolderID: 1, PickRulesID: 1, FolderName: title.Name}
			if err := ins("versions", &version); err != nil {
				return err
			}
			slot := mediafiles.Slot{ItemID: item.ID, VersionID: version.ID, State: "wanted"}
			if title == &movie {
				file := mediafiles.File{VersionID: version.ID, RelativePath: "fresh.mkv", AddedAt: now.Add(-time.Hour), Origin: "imported", PlacedBy: "move", Role: "main", State: "present"}
				if err := ins("files", &file); err != nil {
					return err
				}
				slot.State, slot.FileID = "in_library", &file.ID
			}
			if err := ins("slots", &slot); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	a := get()
	if !slices.Equal(a.Rows, []string{"arrived", "approval", "asks", "discover"}) {
		t.Fatalf("owner rows: %v", a.Rows)
	}
	if len(a.Arrived) != 1 || a.Arrived[0].Name != "Fresh Movie" || a.Arrived[0].Line != "New movie" || !a.Owner {
		t.Fatalf("home answer: %+v", a)
	}
}

// Your Library on Home: one query counts movies, shows (seasons, episodes)
// and artists (albums, tracks), what is on disk, missing or on its way, and
// the bytes on disk. Specials are not a season; dropped episodes and albums
// left out of an artist are not counted; a kind that is on but empty is
// all zeros.
