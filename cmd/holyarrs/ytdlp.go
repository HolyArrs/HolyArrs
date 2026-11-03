package main

import (
	"context"

	"github.com/CoderLuii/holyarrs/internal/api"
	"github.com/CoderLuii/holyarrs/internal/health"
)

// startTrailers feeds the trailers health check from HolyArrs's own
// YouTube reader. Nothing runs yt-dlp any more, so there is no weekly
// yt-dlp update either.
func startTrailers(a *api.API, he *health.Engine) {
	he.Trailers = func(context.Context) health.Trailers {
		s := a.TrailerReader()
		return health.Trailers{On: true, Failed: s.Failed, LastGood: s.LastGood, LastReason: string(s.LastReason)}
	}
}
