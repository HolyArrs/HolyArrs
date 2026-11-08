package api

import (
	"context"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// keepOnDisk stores v under key in lookup_cache with when it was fetched.
func keepOnDisk[T any](ctx context.Context, a *API, key string, v T) {
	metadata.KeepOnDisk(ctx, a.s.Store, key, v)
}

// loadKept reads an answer keepOnDisk stored, as a memory entry.
func loadKept[T any](ctx context.Context, a *API, key string) (struct {
	v  T
	at time.Time
}, bool) {
	var out struct {
		v  T
		at time.Time
	}
	k, ok := metadata.LoadKept[T](ctx, a.s.Store, key)
	out.v, out.at = k.V, k.At
	return out, ok
}

// titleWarmer is discover's title warmer, kept on the API.
type titleWarmer = discover.TitleWarmer

// warmTitles queues titles to be warmed ahead (discover.TitleWarmer).
func (a *API) warmTitles(lang string, titles [][2]string) {
	a.warmSet.Do(func() {
		w := &a.warm
		w.Store = func() *database.Store { return a.s.Store }
		w.Metadata = func() metadata.Provider { return a.s.Metadata }
		w.Cache = func() *discover.Cache { return &a.discover }
		w.WarmExtras = func(ctx context.Context, kind, id string) { a.titles.WarmExtras(ctx, kind, id) }
		w.TrailerKey = func(ctx context.Context, kind, id, lang string) { _, _ = a.trailers.Key(ctx, kind, id, lang) }
	})
	a.warm.Warm(lang, titles)
}
