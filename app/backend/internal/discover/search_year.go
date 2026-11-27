package discover

import (
	"context"
	"strconv"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Search words often end in the year of the title ("Strike 2017"). The
// metadata search asks for every word, so those words find almost nothing:
// the couple of titles that happen to hold the year as well. The year is
// taken off the words and given to the source as a year instead, and what
// that search finds is added to what the words themselves found, so no row
// the words already answered with is lost.

// WithYearMovies adds the movies the words without their trailing year
// find. A search that was given a year keeps it: the year is no longer part
// of the words then.
func WithYearMovies(ctx context.Context, find func(context.Context, string, int) ([]metadata.Movie, error), term string, year int, got []metadata.Movie) []metadata.Movie {
	if year != 0 || find == nil {
		return got
	}
	name, want, ok := mediafiles.SplitYear(term)
	if !ok {
		return got
	}
	more, err := find(ctx, name, want)
	if err != nil {
		return got
	}
	seen := make(map[string]bool, len(got))
	for _, m := range got {
		seen[movieKey(m)] = true
	}
	out := got
	for _, m := range more {
		k := movieKey(m)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, m)
	}
	return out
}

// movieKey is what makes two search rows the same movie.
func movieKey(m metadata.Movie) string {
	if id := m.IDs["tmdb"]; id != "" {
		return "tmdb:" + id
	}
	return mediafiles.Fold(m.Name) + "." + strconv.Itoa(m.Year)
}
