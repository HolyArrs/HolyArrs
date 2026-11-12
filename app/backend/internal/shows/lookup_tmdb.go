package shows

import (
	"context"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// tmdbFinder finds a show's tmdb id from its tvdb or imdb id.
type tmdbFinder interface {
	ShowTMDBID(ctx context.Context, id string) (string, error)
}

// fillTMDBIDs gives results from another details source (TVMaze) their
// tmdb id when TMDB knows the show, so search can offer Add and the
// details page. Misses are left as they are.
func fillTMDBIDs(ctx context.Context, f tmdbFinder, list []metadata.Show) {
	if f == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	var mu sync.Mutex
	sem := make(chan struct{}, 4)
	for i := range list {
		ids := list[i].IDs
		if ids == nil || ids["tmdb"] != "" {
			continue
		}
		var try []string
		if v := ids["tvdb"]; v != "" {
			try = append(try, "tvdb:"+v)
		}
		if v := ids["imdb"]; v != "" {
			try = append(try, "imdb:"+v)
		}
		if len(try) == 0 {
			continue
		}
		wg.Add(1)
		go func(i int, try []string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			for _, id := range try {
				if v, err := f.ShowTMDBID(ctx, id); err == nil && v != "" {
					mu.Lock()
					list[i].IDs["tmdb"] = v
					mu.Unlock()
					return
				}
			}
		}(i, try)
	}
	wg.Wait()
}

// finderFor is the TMDB source when the lookup used another one.
func (h *AddHandlers) finderFor(source string) tmdbFinder {
	if source == "tmdb" || h.Lib == nil || h.Lib.Shows == nil {
		return nil
	}
	p, err := h.Lib.Shows.Get("tmdb")
	if err != nil {
		return nil
	}
	f, _ := p.(tmdbFinder)
	return f
}
