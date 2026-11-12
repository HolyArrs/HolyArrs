package shows

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func addShowService(t *testing.T) (*Service, *database.Store, *fakeShows, *fakeShows) {
	t.Helper()
	s := open(t)
	tmdb, maze := &fakeShows{}, &fakeShows{}
	tmdb.set(labShow("tmdb", "100"), "tmdb:100", "tvdb:200", "imdb:tt0100")
	maze.set(labShow("tvmaze", "300"), "tvmaze:300", "tvdb:200", "imdb:tt0100")
	q := jobs.New(s, jobs.Options{})
	svc := New(&library.Service{Store: s, Queue: q, Shows: metadata.ShowSources{"tmdb": tmdb, "tvmaze": maze, "tvdb": &fakeShows{err: metadata.ErrNoTVDBKey}},
		Now: func() time.Time { return now }})
	return svc, s, tmdb, maze
}

// TestAddShowSeasonChoices is the table of season choices: which seasons
// and episodes are kept up to date right after adding. The lab show has
// specials, a first season that aired in 2025, and a second season whose
// first episode aired yesterday, second airs next week and third has no
// date yet.
func TestAddShowSeasonChoices(t *testing.T) {
	cases := []struct {
		choice   string
		specials bool
		seasons  map[int]bool
		eps      map[string]bool
	}{
		{library.SeasonsAll, false, map[int]bool{0: false, 1: true, 2: true},
			map[string]bool{"S00E01": false, "S01E01": true, "S01E02": true, "S02E01": true, "S02E02": true, "S02E03": true}},
		{library.SeasonsAll, true, map[int]bool{0: true, 1: true, 2: true},
			map[string]bool{"S00E01": true, "S01E01": true, "S02E03": true}},
		{library.SeasonsFuture, false, map[int]bool{0: false, 1: false, 2: true},
			map[string]bool{"S00E01": false, "S01E01": false, "S01E02": false, "S02E01": false, "S02E02": true, "S02E03": true}},
		{library.SeasonsLatest, false, map[int]bool{0: false, 1: false, 2: true},
			map[string]bool{"S00E01": false, "S01E01": false, "S01E02": false, "S02E01": true, "S02E02": true, "S02E03": true}},
		{library.SeasonsNone, false, map[int]bool{0: false, 1: false, 2: false},
			map[string]bool{"S00E01": false, "S01E01": false, "S02E02": false, "S02E03": false}},
	}
	for _, c := range cases {
		name := c.choice
		if c.specials {
			name += "_with_specials"
		}
		t.Run(name, func(t *testing.T) {
			svc, s, _, _ := addShowService(t)
			ctx := context.Background()
			addShowFolder(t, s)
			if c.specials {
				d, _ := library.ReadShowDefaults(ctx, s.Read())
				d.KeepSpecials = true
				if err := SetShowDefaults(ctx, s, d, domain.System); err != nil {
					t.Fatal(err)
				}
			}
			id, jobID, err := svc.AddShow(ctx, "tmdb:100", ShowAddOptions{Seasons: c.choice}, domain.System)
			if err != nil || id == 0 || jobID == 0 {
				t.Fatalf("add %d %d %v", id, jobID, err)
			}
			ss, err := Seasons(ctx, s.Read(), id, now)
			if err != nil || len(ss) != 3 {
				t.Fatalf("seasons %+v %v", ss, err)
			}
			for _, se := range ss {
				if se.KeepUpToDate != c.seasons[se.Number] {
					t.Errorf("season %d keep %v, want %v", se.Number, se.KeepUpToDate, c.seasons[se.Number])
				}
			}
			eps := episodes(t, s, id)
			for k, want := range c.eps {
				if eps[k].Keep != want {
					t.Errorf("%s keep %v, want %v", k, eps[k].Keep, want)
				}
			}
			// The gate does not depend on the choice: aired is wanted.
			if eps["S01E01"].State != mediafiles.SlotWanted || eps["S02E02"].State != mediafiles.SlotNotOutYet {
				t.Errorf("states %s %s", eps["S01E01"].State, eps["S02E02"].State)
			}
			var ti mediafiles.Title
			_ = database.Get(ctx, s.Read(), "titles", id, &ti)
			if ti.Kind != "show" || *ti.SeasonChoice != c.choice || ti.NewPartsPolicy != "all" || *ti.EpisodeOrder != library.OrderAired ||
				ti.MetadataSource != "tmdb" || ti.Status != "running" || ti.SourceIDs["tvdb"] != "200" || ti.RefreshedAt == nil {
				t.Errorf("title %+v", ti)
			}
		})
	}
}

// TestAddShowEpisodeOrders adds the same show once per episode order: the
// rows are the same, the order is stored, absolute numbers and air days
// are there for the absolute and daily orders to use.
func TestAddShowEpisodeOrders(t *testing.T) {
	for _, order := range []string{library.OrderAired, library.OrderDaily, library.OrderAbsolute} {
		t.Run(order, func(t *testing.T) {
			svc, s, _, _ := addShowService(t)
			ctx := context.Background()
			addShowFolder(t, s)
			id, _, err := svc.AddShow(ctx, "tmdb:100", ShowAddOptions{EpisodeOrder: order}, domain.System)
			if err != nil {
				t.Fatal(err)
			}
			var ti mediafiles.Title
			_ = database.Get(ctx, s.Read(), "titles", id, &ti)
			if *ti.EpisodeOrder != order {
				t.Fatalf("order %s", *ti.EpisodeOrder)
			}
			its, _ := database.Select[mediafiles.Item](ctx, s.Read(), "items", "title_id = ? ORDER BY json_extract(numbers, '$.season'), json_extract(numbers, '$.episode')", id)
			if len(its) != 6 {
				t.Fatalf("items %d", len(its))
			}
			want := []struct {
				abs *int
				day string
			}{{nil, "2025-03-01"}, {ip(1), "2025-01-05"}, {ip(2), "2025-01-12"}, {ip(3), "2026-10-11"}, {ip(4), "2026-10-18"}, {ip(5), ""}}
			for i, it := range its {
				day := ""
				if it.AirDay != nil {
					day = *it.AirDay
				}
				if !library.EqInt(it.Numbers.Absolute, want[i].abs) || day != want[i].day {
					t.Errorf("item %d: absolute %v day %q, want %v %q", i, it.Numbers.Absolute, day, want[i].abs, want[i].day)
				}
			}
		})
	}
}

func ip(n int) *int { return &n }

func TestAddShowRefusals(t *testing.T) {
	svc, s, _, maze := addShowService(t)
	ctx := context.Background()
	if _, _, err := svc.AddShow(ctx, "tmdb:100", ShowAddOptions{}, domain.System); !errors.Is(err, library.ErrNoShowFolder) {
		t.Fatalf("no folder: %v", err)
	}
	movies := addFolder(t, s)
	if _, _, err := svc.AddShow(ctx, "tmdb:100", ShowAddOptions{LibraryFolderID: movies.ID}, domain.System); !errors.Is(err, library.ErrNotShowFolder) {
		t.Fatalf("movie folder: %v", err)
	}
	addShowFolder(t, s)
	for _, bad := range []ShowAddOptions{{Seasons: "pilot"}, {NewSeasons: "some"}, {EpisodeOrder: "dvd"}, {Source: "musicbrainz"}} {
		if _, _, err := svc.AddShow(ctx, "tmdb:100", bad, domain.System); err == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
	if _, _, err := svc.AddShow(ctx, "tmdb:999", ShowAddOptions{}, domain.System); !errors.Is(err, metadata.ErrShowNotFound) {
		t.Fatalf("unknown: %v", err)
	}
	if _, _, err := svc.AddShow(ctx, "tvdb:200", ShowAddOptions{Source: "tvdb"}, domain.System); !errors.Is(err, metadata.ErrNoTVDBKey) {
		t.Fatalf("tvdb without key: %v", err)
	}
	// A TMDB id added to follow TVMaze: TVMaze is asked by the TVDB id it learnt.
	id, _, err := svc.AddShow(ctx, "tmdb:100", ShowAddOptions{Source: "tvmaze", NewSeasons: "none"}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	var ti mediafiles.Title
	_ = database.Get(ctx, s.Read(), "titles", id, &ti)
	if ti.MetadataSource != "tvmaze" || ti.SourceIDs["tvmaze"] != "300" || ti.SourceIDs["tmdb"] != "100" || ti.NewPartsPolicy != "none" {
		t.Fatalf("tvmaze show %+v (asked %v)", ti, maze.calls)
	}
	// The same show again, by any of its ids: refused.
	for _, again := range []string{"tmdb:100", "tvdb:200", "tvmaze:300"} {
		var dup *ShowAlreadyAddedError
		if _, _, err := svc.AddShow(ctx, again, ShowAddOptions{Source: "tvmaze"}, domain.System); !errors.As(err, &dup) || dup.TitleID != id {
			t.Fatalf("%s again: %v", again, err)
		}
	}
}

func TestShowSwitchesAndEdit(t *testing.T) {
	svc, s, _, _ := addShowService(t)
	ctx := context.Background()
	addShowFolder(t, s)
	id, _, err := svc.AddShow(ctx, "tmdb:100", ShowAddOptions{}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SetSeason(ctx, id, 1, false, domain.System); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetSeason(ctx, id, 7, true, domain.System); !errors.Is(err, library.ErrNoSuchSeason) {
		t.Fatalf("no season 7: %v", err)
	}
	eps := episodes(t, s, id)
	if eps["S01E01"].Keep || eps["S01E02"].Keep || !eps["S02E01"].Keep {
		t.Fatalf("season switch %+v", eps)
	}
	if it, err := svc.SetEpisode(ctx, eps["S01E02"].ItemID, true, domain.System); err != nil || !it.KeepUpToDate {
		t.Fatalf("episode switch %+v %v", it, err)
	}
	ss, _ := Seasons(ctx, s.Read(), id, now)
	if ss[1].KeepUpToDate || ss[1].Missing != 1 || ss[1].Aired != 2 || ss[2].Missing != 1 || ss[2].NotOutYet != 2 || ss[2].NextAirDate == nil {
		t.Fatalf("summaries %+v", ss)
	}

	// Edit: the Release gate is for movies; order and new seasons are for shows.
	gate := library.GateAnnounced
	if _, err := svc.Edit(ctx, id, library.TitleChange{ReleaseGate: &gate}, false, domain.System); !errors.Is(err, library.ErrMovieOnly) {
		t.Fatalf("gate on a show: %v", err)
	}
	bad := "dvd"
	if _, err := svc.Edit(ctx, id, library.TitleChange{EpisodeOrder: &bad}, false, domain.System); !errors.Is(err, library.ErrBadOrder) {
		t.Fatalf("bad order: %v", err)
	}
	order, none, off := library.OrderAbsolute, "none", false
	if _, err := svc.Edit(ctx, id, library.TitleChange{EpisodeOrder: &order, NewSeasons: &none, KeepUpToDate: &off}, false, domain.System); err != nil {
		t.Fatal(err)
	}
	var ti mediafiles.Title
	_ = database.Get(ctx, s.Read(), "titles", id, &ti)
	after := episodes(t, s, id)
	if *ti.EpisodeOrder != library.OrderAbsolute || ti.NewPartsPolicy != "none" || ti.KeepUpToDate || !after["S02E01"].Keep || after["S02E01"].State != mediafiles.SlotWanted {
		t.Fatalf("after edit %+v %+v", ti, after["S02E01"])
	}
	// Edit on a movie refuses show choices.
	lf := addFolder(t, s)
	mid, err := library.AddMovie(ctx, s, dune, library.AddOptions{LibraryFolderID: lf.ID}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Edit(ctx, mid, library.TitleChange{EpisodeOrder: &order}, false, domain.System); !errors.Is(err, library.ErrShowOnly) {
		t.Fatalf("order on a movie: %v", err)
	}
	// A few episodes at a time from a season pack: 0 is off, 1 to 10 ahead.
	two := 2
	if _, err := svc.Edit(ctx, mid, library.TitleChange{PackWindow: &two}, false, domain.System); !errors.Is(err, library.ErrShowOnly) {
		t.Fatalf("pack window on a movie: %v", err)
	}
	for _, n := range []int{-1, 11} {
		if _, err := svc.Edit(ctx, id, library.TitleChange{PackWindow: &n}, false, domain.System); !errors.Is(err, library.ErrBadPackWindow) {
			t.Fatalf("pack window %d: %v", n, err)
		}
	}
	if _, err := svc.Edit(ctx, id, library.TitleChange{PackWindow: &two}, false, domain.System); err != nil {
		t.Fatal(err)
	}
	_ = database.Get(ctx, s.Read(), "titles", id, &ti)
	if ti.PackWindow != 2 {
		t.Fatalf("pack window saved %d", ti.PackWindow)
	}
	info, err := InfoOf(ctx, s.Read(), ti)
	if err != nil || info.PackWindow != 2 {
		t.Fatalf("pack window read back %+v %v", info, err)
	}
}

func TestShowDefaultsSetting(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	d, err := library.ReadShowDefaults(ctx, s.Read())
	if err != nil || d != (library.ShowDefaults{SeasonChoice: "all", NewSeasons: "all", EpisodeOrder: "aired", Source: "tmdb"}) {
		t.Fatalf("defaults %+v %v", d, err)
	}
	want := library.ShowDefaults{SeasonChoice: "future", NewSeasons: "none", EpisodeOrder: "daily", Source: "tvmaze", KeepSpecials: true, WaitMinutes: 90}
	if err := SetShowDefaults(ctx, s, want, domain.System); err != nil {
		t.Fatal(err)
	}
	if got, _ := library.ReadShowDefaults(ctx, s.Read()); got != want {
		t.Fatalf("back %+v", got)
	}
	for _, bad := range []library.ShowDefaults{{SeasonChoice: "x", NewSeasons: "all", EpisodeOrder: "aired", Source: "tmdb"},
		{SeasonChoice: "all", NewSeasons: "all", EpisodeOrder: "aired", Source: "tmdb", WaitMinutes: -1}} {
		if err := SetShowDefaults(ctx, s, bad, domain.System); err == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
}

// Season summaries count only the main Version's Slots: a second Version
// with every episode on disk does not change them.
func TestSeasonsCountMainVersionOnly(t *testing.T) {
	ctx := context.Background()
	svc, s, _, _ := addShowService(t)
	addShowFolder(t, s)
	id, _, err := svc.AddShow(ctx, "tmdb:100", ShowAddOptions{}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	before, err := Seasons(ctx, s.Read(), id, now)
	if err != nil {
		t.Fatal(err)
	}
	err = s.Write(ctx, func(tx *sql.Tx) error {
		ts, by := database.FormatTime(now), `{"kind":"system"}`
		res, err := tx.Exec(`INSERT INTO versions (created_at, created_by, updated_at, updated_by, title_id, label, pick_rules_id, library_folder_id, folder_name, is_main)
			SELECT ?, ?, ?, ?, title_id, '4K', pick_rules_id, library_folder_id, folder_name, 0 FROM versions WHERE title_id = ?`, ts, by, ts, by, id)
		if err != nil {
			return err
		}
		v, _ := res.LastInsertId()
		_, err = tx.Exec(`INSERT INTO slots (created_at, created_by, updated_at, updated_by, item_id, version_id, state)
			SELECT ?, ?, ?, ?, id, ?, 'in_library' FROM items WHERE title_id = ? AND kind = 'episode'`, ts, by, ts, by, v, id)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	after, err := Seasons(ctx, s.Read(), id, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("seasons %d, want %d", len(after), len(before))
	}
	for i := range before {
		if after[i].InLibrary != before[i].InLibrary || after[i].Missing != before[i].Missing || after[i].NotOutYet != before[i].NotOutYet {
			t.Fatalf("season %d: %+v, want %+v", i, after[i], before[i])
		}
	}
}
