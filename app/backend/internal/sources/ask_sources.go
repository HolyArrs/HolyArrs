package sources

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// askAll asks every Source used for this kind of search; tv picks the Sources
// and categories for shows. A Source with tags is asked only for a title that
// shares one of them.
func (s *Searcher) askAll(ctx context.Context, r *jobs.Run, q Query, hand, tv bool, titleTags []string) ([]found, []string, int, []skippedSource, error) {
	now := time.Now().UTC()
	srcs, err := database.Select[domain.Source](ctx, s.Store.Read(), "sources",
		"state IN ('enabled', 'paused') ORDER BY priority, id")
	if err != nil {
		return nil, nil, 0, nil, err
	}
	var all []found
	var failures []string
	var skipped []skippedSource
	answered := 0
	for i, src := range srcs {
		if src.Format == "rss" || hand && !src.UseFor.HandSearch || !hand && !src.UseFor.AutoSearch || tv && !handlesTV(src) || q.Music && !handlesMusic(src) {
			continue
		}
		if !mediafiles.SourceServes(src.Tags, titleTags) {
			skipped = append(skipped, skippedSource{ID: src.ID, Name: src.Name, Why: tagsWhy + strings.Join(src.Tags, ", ") + ") are not on this title"})
			continue
		}
		if src.State == "paused" && (src.PausedUntil == nil || src.PausedUntil.After(now)) {
			why := "paused"
			if src.PauseReason != nil && *src.PauseReason != "" {
				why = *src.PauseReason
			}
			if b := BlockOf(src); b != "" {
				why = b
			}
			skipped = append(skipped, skippedSource{ID: src.ID, Name: src.Name, Why: why})
			continue
		}
		_ = r.Progress(ctx, float64(i)/float64(len(srcs)+1), "searching "+src.Name)
		if s.onAsk != nil {
			s.onAsk(src.Name)
		}
		qq := q
		if q.Music {
			qq.Categories = musicCategories(src)
		} else if tv {
			qq.Categories = tvCategories(src)
		} else {
			qq.Categories = src.Categories["movie"]
		}
		cl, err := s.client(ctx, src)
		var items []Item
		if err == nil {
			items, err = cl.Search(ctx, qq)
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil, nil, 0, nil, ctx.Err()
			}
			failures = append(failures, src.Name+": "+err.Error())
			skipped = append(skipped, skippedSource{ID: src.ID, Name: src.Name, Why: failWhy(err)})
			s.pause(ctx, src, err)
			continue
		}
		answered++
		s.recovered(ctx, src)
		for _, item := range items {
			all = append(all, found{src: src, item: item, keys: secretValues(cl.cfg)})
		}
	}
	return all, failures, answered, skipped, nil
}

// skippedSource is a Source a search could not use: paused, or failed this
// time. Why is the pause reason or the kind of failure.
type skippedSource struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Why  string `json:"why"`
}

// failWhy names the kind of a Source failure in the pause reason words.
func failWhy(err error) string {
	var se *Error
	// A site's bot check or ban.
	//
	if errors.As(err, &se) && se.Detail != "" {
		return se.Detail
	}
	if errors.As(err, &se) && se.Kind != "" {
		return se.Kind
	}
	return KindDown
}

// noAnswer is the search_failed summary and data when no Source answered. ok
// is false when a Source answered or none was tried. failed is false when
// every Source was only paused: the search is written down but is not a
// failed job.
func noAnswer(failures []string, answered int, skipped []skippedSource) (reason string, data map[string]any, ok, failed bool) {
	if answered > 0 || len(skipped) == 0 {
		return "", nil, false, false
	}
	failed = len(failures) > 0
	tagsOnly := !failed
	if !failed {
		for _, k := range skipped {
			failures = append(failures, k.Name+": "+pauseWords(k.Why))
			tagsOnly = tagsOnly && strings.HasPrefix(k.Why, tagsWhy)
		}
	}
	if tagsOnly {
		return "no Source takes this title's tags: " + strings.Join(failures, "; "), map[string]any{"sources": skipped}, true, false
	}
	return "no Source answered: " + strings.Join(failures, "; "), map[string]any{"sources": skipped}, true, failed
}

// tagsWhy starts the reason a tagged Source was not asked.
const tagsWhy = "its tags ("

// pauseWords says why a Source is paused in plain words.
func pauseWords(why string) string {
	if strings.HasPrefix(why, tagsWhy) {
		return why
	}
	switch why {
	case KindRateLimited:
		return "slowed down"
	case KindDown:
		return "not reachable"
	case KindLoginRefused:
		return "login refused"
	case KindBroken:
		return "setup broken"
	case KindBlocked:
		return "blocked"
	case BlockBotCheck:
		return "blocked by the site's bot check"
	case BlockSiteBan:
		return "the site refuses HolyArrs's address"
	}
	return "paused"
}
