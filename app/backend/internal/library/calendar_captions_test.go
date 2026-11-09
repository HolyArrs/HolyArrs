package library

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// A movie's calendar date carries the words its card shows: genres, length,
// directors, the top-billed cast and its first day out to watch at home.
func TestCalendarMovieDetails(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	addFolder(t, s)
	m := metadata.Movie{Name: "Odyssey", IDs: map[string]string{"tmdb": "9"}, CinemaDate: d("2026-11-05"), DigitalDate: d("2026-12-20"), PhysicalDate: d("2026-12-01"),
		Genres: []string{"Adventure", "Action"}, Runtime: 170, People: []string{"Lead", "Second", "Third", "Fourth", "Boss"}, Directors: []string{"Boss"}, Studio: "Big Pictures"}
	if _, err := AddMovie(ctx, s, m, AddOptions{}, nil, now, domain.System); err != nil {
		t.Fatal(err)
	}
	es, err := Calendar(ctx, s.Read(), *d("2026-11-01"), *d("2026-12-01"))
	if err != nil || len(es) != 1 {
		t.Fatalf("%+v %v", es, err)
	}
	e := es[0]
	if !slices.Equal(e.Genres, m.Genres) || e.Runtime != 170 || e.Studio != "Big Pictures" || !slices.Equal(e.Directors, []string{"Boss"}) || !slices.Equal(e.Cast, []string{"Lead", "Second", "Third"}) ||
		e.CinemaDate == nil || !e.CinemaDate.Equal(*m.CinemaDate) || e.HomeDate == nil || !e.HomeDate.Equal(*m.PhysicalDate) {
		t.Fatalf("%+v", e)
	}
}

// Titles refreshed before directors were kept on their own list them after
// a full cast of ten.
func TestCreditsOfOlderTitles(t *testing.T) {
	people := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "Dir One", "Dir Two"}
	dirs, cast := creditsOf(mediafiles.Title{People: people})
	if !slices.Equal(dirs, []string{"Dir One", "Dir Two"}) || !slices.Equal(cast, []string{"a", "b", "c"}) {
		t.Fatalf("%v %v", dirs, cast)
	}
	// A short list cannot be told apart: no directors.
	if dirs, cast := creditsOf(mediafiles.Title{People: []string{"a", "b"}}); len(dirs) != 0 || !slices.Equal(cast, []string{"a", "b"}) {
		t.Fatalf("%v %v", dirs, cast)
	}
}

// An album's date says its length and its place among the artist's albums.
func TestCalendarAlbumPlace(t *testing.T) {
	svc, s, _, _ := musicService(t)
	ctx := context.Background()
	refreshedLabArtist(t, svc)
	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	es, err := Calendar(ctx, s.Read(), from, from.AddDate(30, 0, 0), CalendarMusic)
	if err != nil || len(es) == 0 {
		t.Fatalf("%+v %v", es, err)
	}
	for _, e := range es {
		if e.Nth < 1 || e.ArtistMBID == "" {
			t.Fatalf("%+v", e)
		}
	}
}
