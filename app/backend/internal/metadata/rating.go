package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
)

// Rater finds the US age rating of a TMDB movie or show with one light
// call, for checking Kid limits without a full details lookup.
type Rater interface {
	Rating(ctx context.Context, kind, tmdbID string) (string, error)
}

// RaterFor returns the rating lookup selected for ctx, when the chosen
// provider has one.
func RaterFor(ctx context.Context, provider Provider) (Rater, bool) {
	if sw, ok := provider.(*Switch); ok {
		provider = sw.pick(ctx)
	}
	r, ok := provider.(Rater)
	return r, ok
}

// Rating is the US rating of a TMDB movie or show; "" when it has none.
// It reads only release_dates (movies) or content_ratings (shows) and
// picks the rating as the details pages do.
func (t *TMDB) Rating(ctx context.Context, kind, tmdbID string) (string, error) {
	n, err := strconv.ParseInt(tmdbID, 10, 64)
	if err != nil || n <= 0 {
		return "", ErrNotFound
	}
	id := strconv.FormatInt(n, 10)
	switch kind {
	case "movie":
		b, err := t.get(ctx, "/movie/"+id+"/release_dates", nil)
		if err != nil {
			return "", err
		}
		var m tmdbMovie
		if err := json.Unmarshal(b, &m.ReleaseDates); err != nil {
			return "", fmt.Errorf("TMDB: %w", err)
		}
		return m.certification(), nil
	case "show":
		b, err := t.get(ctx, "/tv/"+id+"/content_ratings", nil)
		if err != nil {
			return "", err
		}
		var s tmdbShow
		if err := json.Unmarshal(b, &s.ContentRatings); err != nil {
			return "", fmt.Errorf("TMDB: %w", err)
		}
		return s.usRating(), nil
	}
	return "", ErrNotFound
}
