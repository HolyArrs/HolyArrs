package library

import (
	"context"
	"errors"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

type useCacheKey struct{}

// useCache lets a refresh read the lookup cache (the refresh right after an
// add: the add just fetched the same answers).
func useCache(ctx context.Context) context.Context {
	return context.WithValue(ctx, useCacheKey{}, true)
}

// refusedAlbumsStop is how many albums in a row the server may turn down
// before the rest of a refresh waits for the next run.
const refusedAlbumsStop = 3

// serverRefused says the server, not the album, was the problem.
func serverRefused(err error) bool {
	return errors.Is(err, metadata.ErrServiceBusy) || errors.Is(err, metadata.ErrMusicBusy) ||
		errors.Is(err, metadata.ErrServiceUnavailable) || errors.Is(err, metadata.ErrMusicUnavailable)
}
