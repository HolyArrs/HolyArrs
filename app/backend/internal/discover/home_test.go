package discover

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func TestHomeRowOrderPerRole(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
func TestHomeLibraryCounts(t *testing.T) {
	t.Parallel()
	s := homeStore(t)
	ctx := context.Background()
	num := func(n int) *int { return &n }
	err := s.Write(ctx, func(tx *sql.Tx) error {
		ins := func(table string, v any) error { _, err := database.Insert(tx, table, v, domain.System); return err }
		title := func(kind, name string) (mediafiles.Title, mediafiles.Version, error) {
			src := map[string]string{"movie": "tmdb", "show": "tvmaze", "artist": "musicbrainz"}[kind]
			t := mediafiles.Title{Kind: kind, Name: name, SortName: name, MetadataSource: src, WhyAdded: "by_hand", State: "active", NewPartsPolicy: "all", ReleaseGate: "released", GateDriver: "none", Status: "released"}
			if err := ins("titles", &t); err != nil {
				return t, mediafiles.Version{}, err
			}
			v := mediafiles.Version{TitleID: t.ID, Label: "Main", IsMain: true, LibraryFolderID: 1, PickRulesID: 1, FolderName: name}
			return t, v, ins("versions", &v)
		}
		// item adds one item with its main-copy slot; state in_library gets a file of size bytes.
		item := func(t mediafiles.Title, v mediafiles.Version, kind string, part *int64, state string, size int64, dropped bool) error {
			it := mediafiles.Item{TitleID: t.ID, PartID: part, Kind: kind, KeepUpToDate: true}
			if dropped {
				now := time.Now()
				it.DroppedAt = &now
			}
			if err := ins("items", &it); err != nil {
				return err
			}
			slot := mediafiles.Slot{ItemID: it.ID, VersionID: v.ID, State: mediafiles.SlotState(state)}
			if state == "in_library" {
				f := mediafiles.File{VersionID: v.ID, RelativePath: fmt.Sprintf("f%d.mkv", it.ID), Size: size, AddedAt: time.Now(), Origin: "imported", PlacedBy: "move", Role: "main", State: "present"}
				if err := ins("files", &f); err != nil {
					return err
				}
				slot.FileID = &f.ID
			}
			return ins("slots", &slot)
		}
		part := func(t mediafiles.Title, kind string, n *int, included bool) (*int64, error) {
			p := mediafiles.Part{TitleID: t.ID, Kind: kind, Number: n, Name: "p", KeepUpToDate: true, Included: included}
			err := ins("parts", &p)
			return &p.ID, err
		}
		for i, state := range []string{"in_library", "wanted", "downloading"} {
			m, v, err := title("movie", fmt.Sprintf("Movie %d", i))
			if err != nil {
				return err
			}
			if err := item(m, v, "movie", nil, state, 1000, false); err != nil {
				return err
			}
		}
		sh, sv, err := title("show", "Show")
		if err != nil {
			return err
		}
		specials, _ := part(sh, "season", num(0), true)
		s1, _ := part(sh, "season", num(1), true)
		s2, err := part(sh, "season", num(2), true)
		if err != nil {
			return err
		}
		// A dropped season and its episode on disk are not counted.
		s3, err := part(sh, "season", num(3), true)
		if err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE parts SET dropped_at = ? WHERE id = ?", database.FormatTime(time.Now()), *s3); err != nil {
			return err
		}
		if err := item(sh, sv, "episode", s3, "in_library", 500, false); err != nil {
			return err
		}
		for _, e := range []struct {
			part    *int64
			state   string
			dropped bool
		}{{specials, "in_library", false}, {s1, "in_library", false}, {s1, "in_library", false}, {s2, "wanted", false}, {s2, "not_out_yet", false}, {s2, "wanted", true}} {
			if err := item(sh, sv, "episode", e.part, e.state, 500, e.dropped); err != nil {
				return err
			}
		}
		ar, av, err := title("artist", "Artist")
		if err != nil {
			return err
		}
		kept, _ := part(ar, "album", nil, true)
		left, err := part(ar, "album", nil, false)
		if err != nil {
			return err
		}
		for _, e := range []struct {
			part  *int64
			state string
		}{{kept, "in_library"}, {kept, "searching"}, {left, "wanted"}} {
			if err := item(ar, av, "track", e.part, e.state, 10, false); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := homeLibraryCounts(ctx, s.Read(), "t.state != 'removed' AND t.kind IN ('movie', 'show', 'artist')", nil, []string{"movie", "show", "artist"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]homeLibraryKind{
		"movie":  {Titles: 3, Items: 3, OnDisk: 1, Missing: 1, Downloading: 1, Size: 1000},
		"show":   {Titles: 1, Parts: 2, Items: 5, OnDisk: 3, Missing: 1, Size: 1500},
		"artist": {Titles: 1, Parts: 1, Items: 2, OnDisk: 1, Missing: 1, Size: 10},
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s: %+v, want %+v", k, got[k], w)
		}
	}
	// Only the kinds asked for, and one with nothing in the filter is zeros.
	got, err = homeLibraryCounts(ctx, s.Read(), "t.state != 'removed' AND t.kind IN ('movie')", nil, []string{"movie", "show"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got["movie"].Titles != 3 || got["show"] != (homeLibraryKind{}) {
		t.Errorf("movies only: %+v", got)
	}
}

// homeStore is a fresh database with one Library folder (id 1).
func homeStore(t *testing.T) *database.Store {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	err = s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := database.Insert(tx, "library_folders", &mediafiles.LibraryFolder{Path: t.TempDir(), Kind: "movie", Label: "Movies", Health: "ok", SameDiskAs: []int64{}}, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestHomeRowsOverHTTP(t *testing.T) {
	t.Parallel()
	s := homeStore(t)
	m := http.NewServeMux()
	testHandlers(s, metadata.Lab()).Routes(m)
	srv := httptest.NewServer(m)
	t.Cleanup(srv.Close)
	type answer struct {
		Rows       []string   `json:"rows"`
		Owner      bool       `json:"owner"`
		Arrived    []HomeCard `json:"arrived"`
		NewSeasons []HomeCard `json:"new_seasons"`
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
