package source

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/judge"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/tagset"
)

const feedProbeTimeout = 45 * time.Second
const feedProbeParallel = 8

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

type feedPrepared struct {
	answers     []feedProbeAnswer
	fingerprint [sha256.Size]byte
	showsWanted bool
	musicWanted bool
}

func feedSourceFingerprint(srcs []domain.Source) [sha256.Size]byte {
	// The digest binds prepared answers to the complete stored Source
	// revisions without retaining a decrypted key in the Job or logs.
	b, _ := json.Marshal(srcs)
	return sha256.Sum256(b)
}

// ProbeFeed reads feeds and reports whether the durable feed Job has a
// meaningful change to persist. It shares each Source client's request
// spacing, but does not record call health, change Source state, add Events,
// or advance a cursor. Any uncertainty asks the durable Job to run.
func (s *Searcher) ProbeFeed(ctx context.Context, now time.Time) (bool, any, error) {
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
			cats := src.Categories["movie"]
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

	hasShow := slices.ContainsFunc(refs, func(r judge.TitleRef) bool { return r.Kind == "show" })
	hasArtist := slices.ContainsFunc(refs, func(r judge.TitleRef) bool { return r.Kind == "artist" })
	for _, answer := range answers {
		if !answer.src.UseFor.Feed {
			continue
		}
		if answer.err != nil || answer.recovery {
			return true, prepared, nil
		}
		for _, item := range answer.items {
			var seen bool
			if err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM releases
				WHERE source_id = ? AND name = ? AND (found_by = 'feed' OR state = 'judged'))`,
				answer.src.ID, item.Title).Scan(&seen); err != nil {
				return true, nil, err
			}
			if seen {
				continue
			}
			p := releasename.Parse(item.Title, releasename.Hints{Size: item.Size})
			m := judge.MatchTitle(p, refs)
			if m.Title == nil && hasShow {
				m = judge.MatchTitle(releasename.Parse(item.Title, releasename.Hints{Size: item.Size, Show: true}), refs)
			}
			if m.Title == nil && hasArtist {
				m = judge.MatchTitle(releasename.Parse(item.Title, releasename.Hints{Size: item.Size, Music: true}), refs)
			}
			if m.Title != nil && tagset.SourceServes(answer.src.Tags, m.Title.Tags) {
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
