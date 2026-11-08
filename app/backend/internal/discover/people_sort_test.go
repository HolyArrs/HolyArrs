package discover

import (
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func TestSortPeopleMoviesOldestFirstUpcomingLast(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	day := func(y int) *time.Time { d := time.Date(y, 3, 1, 0, 0, 0, 0, time.UTC); return &d }
	c := func(id string, year int, date *time.Time, pop float64) metadata.PersonMovieCredit {
		return metadata.PersonMovieCredit{Movie: metadata.Movie{Name: id, Year: year, CinemaDate: date, IDs: map[string]string{"tmdb": id}}, Popularity: pop}
	}
	movies := []metadata.PersonMovieCredit{c("soon", 2027, day(2027), 9), c("b", 2015, day(2015), 1), c("undated", 0, nil, 5), c("a", 2005, day(2005), 3)}
	ids := func() (out []string) {
		for _, m := range movies {
			out = append(out, m.Movie.IDs["tmdb"])
		}
		return
	}
	sortPeopleMovies(movies, "oldest", now)
	if got := ids(); got[0] != "a" || got[1] != "b" {
		t.Fatalf("oldest: %v", got)
	}
	sortPeopleMovies(movies, "newest", now)
	if got := ids(); got[0] != "b" || got[1] != "a" {
		t.Fatalf("newest: %v", got)
	}
	sortPeopleMovies(movies, "popular", now)
	if got := ids(); got[0] != "soon" || got[3] != "b" {
		t.Fatalf("popular: %v", got)
	}
}
