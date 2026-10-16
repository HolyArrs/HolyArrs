package metadata

import (
	"context"
	"errors"
	"time"
)

// Switch asks Use on every lookup: true sends it to Then, false to Else. It
// lets the app use TMDB once a token is set and the fixed lab record before.
type Switch struct {
	Use  func(ctx context.Context) bool
	Then Provider
	Else Provider
}

func (s *Switch) pick(ctx context.Context) Provider {
	if s.Use(ctx) {
		return s.Then
	}
	return s.Else
}

// Search finds movies with the chosen provider.
func (s *Switch) Search(ctx context.Context, query string, year int) ([]Movie, error) {
	return s.pick(ctx).Search(ctx, query, year)
}

// Movie looks one movie up with the chosen provider.
func (s *Switch) Movie(ctx context.Context, id string) (Movie, error) {
	return s.pick(ctx).Movie(ctx, id)
}

// ErrNoChangeList means the provider cannot list changed movies.
var ErrNoChangeList = errors.New("this metadata provider has no changed list")

// Changed lists changed movies with the chosen provider, when it can.
func (s *Switch) Changed(ctx context.Context, since time.Time) ([]string, error) {
	if cl, ok := s.pick(ctx).(ChangeLister); ok {
		return cl.Changed(ctx, since)
	}
	return nil, ErrNoChangeList
}
