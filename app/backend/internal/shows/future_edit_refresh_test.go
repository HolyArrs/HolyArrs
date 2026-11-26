package shows

import (
	"context"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func TestRefreshShow_ShouldMonitorOnlyFutureEpisodesAndNewSeasons_WhenEditedToFuture(t *testing.T) {
	svc, s, tmdb, _ := addShowService(t)
	ctx := context.Background()
	addShowFolder(t, s)

	id, _, err := svc.AddShow(ctx, "tmdb:100", ShowAddOptions{Seasons: library.SeasonsAll}, domain.System)
	if err != nil {
		t.Fatal(err)
	}

	fut := library.SeasonsFuture
	if _, err := svc.Edit(ctx, id, library.TitleChange{SeasonChoice: &fut}, false, domain.System); err != nil {
		t.Fatal(err)
	}

	sh := labShow("tmdb", "100")
	sh.Seasons = append(sh.Seasons, metadata.Season{Number: 3, Name: "Season 3"})
	sh.Episodes = append(sh.Episodes,
		metadata.Episode{Season: 1, Number: 3, Name: "Found late", AirDate: tm("2025-01-20T02:00:00Z"), ID: "tmdb-103"},
		metadata.Episode{Season: 3, Number: 1, Name: "New season", AirDate: tm("2027-01-10T02:00:00Z"), ID: "tmdb-301"})
	tmdb.set(sh, "tmdb:100", "tvdb:200", "imdb:tt0100")

	if _, err := svc.RefreshShow(ctx, id, domain.System, nil); err != nil {
		t.Fatal(err)
	}

	eps := episodes(t, s, id)
	if eps["S01E03"].Keep || !eps["S03E01"].Keep || eps["S02E01"].Keep || !eps["S02E02"].Keep {
		t.Fatalf("unexpected keep flags: S01E03=%v S03E01=%v S02E01=%v S02E02=%v",
			eps["S01E03"].Keep, eps["S03E01"].Keep, eps["S02E01"].Keep, eps["S02E02"].Keep)
	}

	ss, err := Seasons(ctx, s.Read(), id, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, se := range ss {
		if want := se.Number >= 2; se.KeepUpToDate != want {
			t.Errorf("season %d keep_up_to_date=%v, want %v", se.Number, se.KeepUpToDate, want)
		}
	}
}
