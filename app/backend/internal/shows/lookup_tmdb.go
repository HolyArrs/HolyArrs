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

// copiedTMDB says a show's TMDB id is its own TVDB or TVmaze id: a number
// copied into the wrong field, which on TMDB names another show.
func copiedTMDB(ids map[string]string) bool {
	v := ids["tmdb"]
	return v != "" && (v == ids["tvdb"] || v == ids["tvmaze"])
}

// fixCopiedTMDB drops such an id from an answer of another source and
// looks the show's real TMDB id up on TMDB by its TVDB or IMDb id. TMDB's
// own answers are left as they are.
func (s *Service) fixCopiedTMDB(ctx context.Context, sh *metadata.Show) {
	if sh.Source == "tmdb" || !copiedTMDB(sh.IDs) {
		return
	}
	delete(sh.IDs, "tmdb")
	p, err := s.showSource("tmdb")
	if err != nil {
		return
	}
	for _, k := range []string{"tvdb", "imdb"} {
		if v := sh.IDs[k]; v != "" {
			if m, err := p.Show(ctx, k+":"+v); err == nil && m.IDs["tmdb"] != "" && metadata.SameShow(*sh, m) {
				sh.IDs["tmdb"] = m.IDs["tmdb"]
				return
			}
		}
	}
}

// lookupKnown is lookupShow for a show already stored. A stored TMDB id
// that is a copied TVDB or TVmaze id is not used, and a TMDB answer whose
// TVDB id is not the stored one (the stored TMDB id is another show's) is
// replaced by the show found by its other ids, so a wrong TMDB id never
// turns the show into another one.
func (s *Service) lookupKnown(ctx context.Context, p metadata.ShowProvider, source string, ids map[string]string) (metadata.Show, error) {
	others := map[string]string{}
	for k, v := range ids {
		if k != "tmdb" {
			others[k] = v
		}
	}
	if copiedTMDB(ids) {
		return lookupShow(ctx, p, source, others)
	}
	sh, err := lookupShow(ctx, p, source, ids)
	if err == nil && sh.Source == "tmdb" && ids["tmdb"] != "" && ids["tvdb"] != "" && sh.IDs["tvdb"] != "" && sh.IDs["tvdb"] != ids["tvdb"] {
		if alt, err := lookupShow(ctx, p, source, others); err == nil && alt.IDs["tmdb"] != "" {
			return alt, nil
		}
	}
	return sh, err
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
