package library

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func TestEditSeasonChoiceFuture_ShouldKeepOnlyUnairedAndEmptySeasons_WhenSwitchedToFuture(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	sh := seedLibShow(t, s, "MobLand")
	// Add an empty upcoming placeholder season (Season 3) and specials (Season 0).
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		for _, se := range []struct {
			num  int
			name string
		}{{0, "Specials"}, {3, "Season 3"}} {
			n := se.num
			if _, err := database.Insert(tx, "parts", &mediafiles.Part{
				TitleID: sh.Title, Kind: "season", Number: &n, Name: se.name,
				KeepUpToDate: false, SourceIDs: map[string]string{}, Pressings: []mediafiles.Pressing{},
			}, domain.System); err != nil {
				return err
			}
		}
		if _, err := tx.Exec("UPDATE titles SET new_parts_policy = 'none' WHERE id = ?", sh.Title); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	svc, _, _ := runQueue(t, s, &metadata.Static{})

	fut := SeasonsFuture
	if _, err := svc.Edit(ctx, sh.Title, TitleChange{SeasonChoice: &fut}, true, domain.System); err != nil {
		t.Fatal(err)
	}
	var ti mediafiles.Title
	if err := database.Get(ctx, s.Read(), "titles", sh.Title, &ti); err != nil {
		t.Fatal(err)
	}
	if ti.FutureFrom != nil || (ti.SeasonChoice != nil && *ti.SeasonChoice == SeasonsFuture) {
		t.Fatalf("dry run mutated title: %+v", ti)
	}

	if _, err := svc.Edit(ctx, sh.Title, TitleChange{SeasonChoice: &fut}, false, domain.System); err != nil {
		t.Fatal(err)
	}
	if err := database.Get(ctx, s.Read(), "titles", sh.Title, &ti); err != nil {
		t.Fatal(err)
	}
	if ti.SeasonChoice == nil || *ti.SeasonChoice != SeasonsFuture || ti.NewPartsPolicy != "all" || ti.FutureFrom == nil || !ti.FutureFrom.Equal(now) {
		t.Fatalf("unexpected title state after future edit: %+v", ti)
	}

	parts, err := database.Select[mediafiles.Part](ctx, s.Read(), "parts", "title_id = ? AND kind = 'season'", sh.Title)
	if err != nil {
		t.Fatal(err)
	}
	wantParts := map[int]bool{0: false, 1: false, 2: true, 3: true}
	for _, p := range parts {
		if p.Number != nil && p.KeepUpToDate != wantParts[*p.Number] {
			t.Errorf("season %d keep_up_to_date=%v, want %v", *p.Number, p.KeepUpToDate, wantParts[*p.Number])
		}
	}

	wantEps := map[string]bool{"S01E01": false, "S01E02": false, "S02E01": false, "S02E02": true}
	for k, want := range wantEps {
		var it mediafiles.Item
		if err := database.Get(ctx, s.Read(), "items", sh.Eps[k].Item, &it); err != nil {
			t.Fatal(err)
		}
		if it.KeepUpToDate != want {
			t.Errorf("%s keep_up_to_date=%v, want %v", k, it.KeepUpToDate, want)
		}
	}
}

func TestEditSeasonChoiceFuture_ShouldClearFutureFrom_WhenSwitchedAwayFromFuture(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	sh := seedLibShow(t, s, "Star City")
	svc, _, _ := runQueue(t, s, &metadata.Static{})

	fut := SeasonsFuture
	if _, err := svc.Edit(ctx, sh.Title, TitleChange{SeasonChoice: &fut}, false, domain.System); err != nil {
		t.Fatal(err)
	}

	for _, choice := range []string{SeasonsLatest, SeasonsAll, SeasonsNone} {
		c := choice
		if _, err := svc.Edit(ctx, sh.Title, TitleChange{SeasonChoice: &c}, false, domain.System); err != nil {
			t.Fatalf("edit %s: %v", choice, err)
		}
		var ti mediafiles.Title
		if err := database.Get(ctx, s.Read(), "titles", sh.Title, &ti); err != nil {
			t.Fatal(err)
		}
		if ti.SeasonChoice == nil || *ti.SeasonChoice != choice {
			t.Fatalf("got season_choice %v, want %s", ti.SeasonChoice, choice)
		}
		if ti.FutureFrom != nil {
			t.Fatalf("expected future_from cleared for %s, got %v", choice, ti.FutureFrom)
		}
	}
}

func TestEditSeasonChoiceFuture_ShouldRejectInvalidAndMovie(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	sh := seedLibShow(t, s, "Star City")
	svc, _, _ := runQueue(t, s, &metadata.Static{})

	bad := "pilot"
	if _, err := svc.Edit(ctx, sh.Title, TitleChange{SeasonChoice: &bad}, false, domain.System); !errors.Is(err, ErrBadSeasonChoice) {
		t.Fatalf("got %v, want ErrBadSeasonChoice", err)
	}

	addFolder(t, s)
	movieID, err := AddMovie(ctx, s, dune, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	fut := SeasonsFuture
	if _, err := svc.Edit(ctx, movieID, TitleChange{SeasonChoice: &fut}, false, domain.System); !errors.Is(err, ErrShowOnly) {
		t.Fatalf("got %v, want ErrShowOnly", err)
	}
}
