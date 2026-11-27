package shows

import (
	"context"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// WithYear adds the shows the words without their trailing year find, a
// show of that year first. Search words often end in the year of the show
// ("Strike 2017"); a source asked for every word finds no show at all then,
// and the year belongs in the year, not in the name.
func WithYear(ctx context.Context, find func(context.Context, string) ([]metadata.Show, error), term string, got []metadata.Show) []metadata.Show {
	if find == nil {
		return got
	}
	name, year, ok := mediafiles.SplitYear(term)
	if !ok {
		return got
	}
	more, err := find(ctx, name)
	if err != nil {
		return got
	}
	seen := make(map[string]bool, len(got))
	for _, sh := range got {
		seen[showKey(sh)] = true
	}
	ofYear := []metadata.Show{}
	rest := []metadata.Show{}
	for _, sh := range more {
		k := showKey(sh)
		if seen[k] {
			continue
		}
		seen[k] = true
		if sh.Year == year {
			ofYear = append(ofYear, sh)
		} else {
			rest = append(rest, sh)
		}
	}
	return append(append(got, ofYear...), rest...)
}

// showKey is what makes two search rows the same show.
func showKey(sh metadata.Show) string {
	for _, k := range []string{"tmdb", "tvdb", "tvmaze", "imdb"} {
		if id := sh.IDs[k]; id != "" {
			return k + ":" + id
		}
	}
	return mediafiles.Fold(sh.Name)
}
