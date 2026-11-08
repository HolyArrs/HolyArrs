package main

import (
	"context"

	"github.com/HolyArrs/holyarrs/app/backend/internal/health"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// startTrailers feeds the trailers health check from HolyArrs's own
// YouTube reader: the different titles and music videos that failed since
// the last good lookup, as kept before a restart (read here, at start).
// With trailer autoplay off nothing is looked up, so the check is off too
// rather than showing a kept failure nothing can clear.
func startTrailers(t *metadata.Trailers, he *health.Engine) {
	_ = t.ReaderState()
	he.Trailers = func(ctx context.Context) health.Trailers {
		s := t.ReaderState()
		return health.Trailers{On: t.On(ctx), Failed: s.Videos, LastGood: s.LastGood, LastReason: string(s.LastReason)}
	}
}
