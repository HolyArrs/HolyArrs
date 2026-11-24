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

func TestEditSeasonChoice_ShouldPersistAndClearFutureFrom_WhenShowUpdated(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	sh := seedLibShow(t, s, "Star City")
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE titles SET season_choice = 'future', future_from = ? WHERE id = ?", database.FormatTime(now), sh.Title)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	svc, _, _ := runQueue(t, s, &metadata.Static{})

	latest := SeasonsLatest
	if _, err := svc.Edit(ctx, sh.Title, TitleChange{SeasonChoice: &latest}, true, domain.System); err != nil {
		t.Fatal(err)
	}
	var ti mediafiles.Title
	if err := database.Get(ctx, s.Read(), "titles", sh.Title, &ti); err != nil {
		t.Fatal(err)
	}
	if ti.SeasonChoice == nil || *ti.SeasonChoice != SeasonsFuture || ti.FutureFrom == nil {
		t.Fatalf("dry run mutated title: choice=%v future_from=%v", ti.SeasonChoice, ti.FutureFrom)
	}

	for _, choice := range []string{SeasonsLatest, SeasonsAll, SeasonsNone} {
		c := choice
		if _, err := svc.Edit(ctx, sh.Title, TitleChange{SeasonChoice: &c}, false, domain.System); err != nil {
			t.Fatalf("edit %s: %v", choice, err)
		}
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

func TestEditSeasonChoice_ShouldRejectInvalidChoice_WhenChoiceUnknown(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	sh := seedLibShow(t, s, "Star City")
	svc, _, _ := runQueue(t, s, &metadata.Static{})

	bad := "pilot"
	if _, err := svc.Edit(ctx, sh.Title, TitleChange{SeasonChoice: &bad}, false, domain.System); !errors.Is(err, ErrBadSeasonChoice) {
		t.Fatalf("got %v, want ErrBadSeasonChoice", err)
	}
}

func TestEditSeasonChoice_ShouldRejectMovie_WhenTitleIsNotShow(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	addFolder(t, s)
	svc, _, _ := runQueue(t, s, &metadata.Static{Movies: []metadata.Movie{dune}})
	movieID, err := AddMovie(ctx, s, dune, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}

	latest := SeasonsLatest
	if _, err := svc.Edit(ctx, movieID, TitleChange{SeasonChoice: &latest}, false, domain.System); !errors.Is(err, ErrShowOnly) {
		t.Fatalf("got %v, want ErrShowOnly", err)
	}
}
