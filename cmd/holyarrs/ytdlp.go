package main

import (
	"context"

	"github.com/CoderLuii/holyarrs/internal/api"
	"github.com/CoderLuii/holyarrs/internal/health"
)

// startTrailers feeds the trailers health check from HolyArrs's own
// YouTube reader: the different titles and music videos that failed since
// the last good lookup, as kept before a restart (read here, at start).
// With trailer autoplay off nothing is looked up, so the check is off too
// rather than showing a kept failure nothing can clear. Nothing runs
// yt-dlp any more, so there is no weekly yt-dlp update either.
func startTrailers(a *api.API, he *health.Engine) {
	_ = a.TrailerReader()
	he.Trailers = func(ctx context.Context) health.Trailers {
		s := a.TrailerReader()
		return health.Trailers{On: a.TrailersOn(ctx), Failed: s.Videos, LastGood: s.LastGood, LastReason: string(s.LastReason)}
	}
}
