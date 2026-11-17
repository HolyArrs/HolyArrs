package sources

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// feedProbeTimeout bounds one whole probe so a slow Source cannot stall the scheduler.
const feedProbeTimeout = 45 * time.Second

// feedProbeParallel is how many Sources the probe reads at once.
const feedProbeParallel = 8

// feedProbeAnswer is what one Source returned during a probe.
type feedProbeAnswer struct {
	src        domain.Source
	items      []Item
	keys       []string
	observed   bool
	observedAt time.Time
	callTook   time.Duration
	callErr    error
	recovery   bool
	err        error
}

// feedPrepared carries the probe answers to the feed check, so the check does not read the Sources a second time.
type feedPrepared struct {
	answers     []feedProbeAnswer
	fingerprint [sha256.Size]byte
	showsWanted bool
	musicWanted bool
}

// ProbeFeed reports whether a feed check would change anything. It reads the feeds once and, when work is found, hands the answers on as the prepared value. When nothing matches, the quiet calls are only remembered. While the Sources part is off it finds nothing to do.
func (s *Searcher) ProbeFeed(ctx context.Context, now time.Time) (bool, any, error) {
	if !s.on() {
		return false, nil, nil
	}
	if due, _, err := s.ProbeGrabHeld(ctx, now); err != nil || due {
		return true, nil, err
	}
	db := s.Store.Read()
	srcs, err := database.Select[domain.Source](ctx, db, "sources",
		"(state = 'enabled' OR (state = 'paused' AND paused_until <= ?)) ORDER BY priority, id", database.FormatTime(now.UTC()))
	if err != nil {
		return true, nil, err
	}
	refs, err := s.wantedTitleRefs(ctx)
	if err != nil {
		return true, nil, err
	}
	var showsWanted, musicWanted bool
	for _, ref := range refs {
		showsWanted = showsWanted || ref.Kind == "show"
		musicWanted = musicWanted || ref.Kind == "artist"
	}

	// A failed read, a recovering Source or a changed Source list all count as work, so the real check can record them.
	probeCtx, cancel := context.WithTimeout(ctx, feedProbeTimeout)
	defer cancel()
	answers := make([]feedProbeAnswer, len(srcs))
	sem := make(chan struct{}, feedProbeParallel)
	var wg sync.WaitGroup
	for i, src := range srcs {
		if !src.UseFor.Feed {
			continue
		}
		wg.Add(1)
		go func(i int, src domain.Source) {
			defer wg.Done()
			defer func() {
				if recover() != nil {
					answers[i] = feedProbeAnswer{src: src, err: errors.New("feed probe worker panicked")}
				}
			}()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-probeCtx.Done():
				answers[i] = feedProbeAnswer{src: src, err: probeCtx.Err()}
				return
			}
			cl, err := s.client(probeCtx, src)
			if err != nil {
				answers[i] = feedProbeAnswer{src: src, err: err}
				return
			}
			cats := movieFeedCategories(src)
			if handlesTV(src) && showsWanted {
				cats = append(slices.Clone(cats), tvCategories(src)...)
			}
			if handlesMusic(src) && musicWanted {
				cats = append(slices.Clone(cats), musicCategories(src)...)
			}
			answer := feedProbeAnswer{src: src, keys: secretValues(cl.cfg), recovery: src.State == "paused"}
			answer.items, answer.err = cl.feedProbe(probeCtx, cats, func(_ string, took time.Duration, err error) {
				answer.observed, answer.observedAt, answer.callTook, answer.callErr = true, time.Now().UTC(), took, err
			})
			_, failedBefore := src.Extra["failures"]
			answer.recovery = answer.recovery || failedBefore
			answers[i] = answer
		}(i, src)
	}
	wg.Wait()
	if err := probeCtx.Err(); err != nil {
		return true, nil, err
	}
	prepared := &feedPrepared{answers: answers, fingerprint: feedSourceFingerprint(srcs), showsWanted: showsWanted, musicWanted: musicWanted}

	hasShow := slices.ContainsFunc(refs, func(r mediafiles.TitleRef) bool { return r.Kind == "show" })
	hasArtist := slices.ContainsFunc(refs, func(r mediafiles.TitleRef) bool { return r.Kind == "artist" })
	for _, answer := range answers {
		if !answer.src.UseFor.Feed {
			continue
		}
		if answer.err != nil || answer.recovery {
			return true, prepared, nil
		}
		for _, item := range answer.items {
			seen, _, err := releaseSeen(ctx, db, answer.src.ID, item.Title)
			if err != nil {
				return true, nil, err
			}
			if seen {
				continue
			}
			p := releasename.Parse(item.Title, releasename.Hints{Size: item.Size})
			m := mediafiles.MatchTitle(p, refs)
			if m.Title == nil && hasShow {
				m = mediafiles.MatchTitle(releasename.Parse(item.Title, releasename.Hints{Size: item.Size, Show: true}), refs)
			}
			if m.Title == nil && hasArtist {
				m = mediafiles.MatchTitle(releasename.Parse(item.Title, releasename.Hints{Size: item.Size, Music: true}), refs)
			}
			if m.Title != nil && mediafiles.SourceTakesTitle(answer.src.Tags, m.Title.Tags, m.Title.RuleTags) {
				return true, prepared, nil
			}
		}
	}
	current, err := database.Select[domain.Source](ctx, db, "sources",
		"(state = 'enabled' OR (state = 'paused' AND paused_until <= ?)) ORDER BY priority, id", database.FormatTime(now.UTC()))
	if err != nil {
		return true, nil, err
	}
	if feedSourceFingerprint(current) != prepared.fingerprint {
		return true, prepared, nil
	}
	for _, answer := range answers {
		if answer.observed && answer.callErr == nil {
			s.rememberQuietFeedCall(ctx, answer.src, answer.observedAt, answer.callTook)
		}
	}
	return false, nil, nil
}

// feedSourceFingerprint hashes the Source list so the feed check can tell the probe answers are still for the same Sources.
func feedSourceFingerprint(srcs []domain.Source) [sha256.Size]byte {

	b, _ := json.Marshal(srcs)
	return sha256.Sum256(b)
}
