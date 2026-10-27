package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/store"
)

func TestHomeRowOrderPerRole(t *testing.T) {
	full := homeHas{Attention: true, Arrived: true, NewSeason: true, Following: true, Soon: true}
	cases := []struct {
		name string
		v    homeViewer
		has  homeHas
		want []string
	}{
		{"member", homeViewer{}, full, []string{"arrived", "asks", "new_season", "following", "soon", "discover"}},
		{"owner", homeViewer{Owner: true, Approver: true}, full, []string{"attention", "arrived", "approval", "asks", "new_season", "following", "soon", "discover"}},
		{"kid", homeViewer{Kid: true}, full, []string{"arrived", "asks", "new_season"}},
		{"empty member", homeViewer{}, homeHas{Empty: true}, []string{"welcome"}},
		{"empty owner", homeViewer{Owner: true, Approver: true}, homeHas{Empty: true, Attention: true}, []string{"welcome"}},
		{"owner, nothing failed", homeViewer{Owner: true, Approver: true}, homeHas{Arrived: true}, []string{"arrived", "approval", "asks", "discover"}},
		{"member, no follows", homeViewer{}, homeHas{Arrived: true, NewSeason: true, Soon: true}, []string{"arrived", "asks", "new_season", "soon", "discover"}},
	}
	for _, c := range cases {
		if got := homeRows(c.v, c.has); !slices.Equal(got, c.want) {
			t.Errorf("%s: rows %v, want %v", c.name, got, c.want)
		}
	}
}

func TestHomeShowLine(t *testing.T) {
	for _, c := range []struct {
		newShow                 bool
		season, fresh, inSeason int
		want                    string
	}{
		{true, 1, 8, 8, "New show"},
		{false, 5, 10, 10, "Season 5 added"},
		{false, 5, 3, 10, "S5 · 3 new episodes"},
		{false, 2, 1, 6, "S2 · 1 new episode"},
	} {
		if got := homeShowLine(c.newShow, c.season, c.fresh, c.inSeason); got != c.want {
			t.Errorf("%+v: %q", c, got)
		}
	}
	if got := homeArtistLine(3, make([]homeAlbum, 3)); got != "3 new albums" {
		t.Errorf("artist line %q", got)
	}
}

// route: GET /api/v1/home
func TestHomeRowsOverHTTP(t *testing.T) {
	srv, s, _ := setup(t)
	type answer struct {
		Rows       []string   `json:"rows"`
		Owner      bool       `json:"owner"`
		Arrived    []homeCard `json:"arrived"`
		NewSeasons []homeCard `json:"new_seasons"`
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
		ins := func(table string, v any) error { _, err := store.Insert(tx, table, v, domain.System); return err }
		movie := domain.Title{Kind: "movie", Name: "Fresh Movie", SortName: "fresh movie", MetadataSource: "tmdb", WhyAdded: "by_hand", State: "active", NewPartsPolicy: "all", ReleaseGate: "released", GateDriver: "none", Status: "released"}
		wanted := domain.Title{Kind: "movie", Name: "Wanted Movie", SortName: "wanted movie", MetadataSource: "tmdb", WhyAdded: "by_hand", State: "active", NewPartsPolicy: "all", ReleaseGate: "released", GateDriver: "none", Status: "released"}
		for _, title := range []*domain.Title{&movie, &wanted} {
			if err := ins("titles", title); err != nil {
				return err
			}
			item := domain.Item{TitleID: title.ID, Kind: "movie", Name: &title.Name}
			if err := ins("items", &item); err != nil {
				return err
			}
			version := domain.Version{TitleID: title.ID, Label: "Main", IsMain: true, LibraryFolderID: 1, PickRulesID: 1, FolderName: title.Name}
			if err := ins("versions", &version); err != nil {
				return err
			}
			slot := domain.Slot{ItemID: item.ID, VersionID: version.ID, State: "wanted"}
			if title == &movie {
				file := domain.File{VersionID: version.ID, RelativePath: "fresh.mkv", AddedAt: now.Add(-time.Hour), Origin: "imported", PlacedBy: "move", Role: "main", State: "present"}
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
