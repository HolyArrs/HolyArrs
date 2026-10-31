package dlsync

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/store"
)

// MatchFunc looks one client item up and links it when sure. It must never
// add to the Library (Phase 2 ships link-only). outcome is one of linked,
// suggested, unsure, not_media or own; an error is retried later.
type MatchFunc func(ctx context.Context, downloaderID int64, key, name, category string) (outcome string, titleID int64, err error)

// Match queue pacing.
const (
	matchRate  = 2.0 // lookups per second, shared by every client
	matchBurst = 10
	matchTries = 6 // failures before nomatch
)

// matchBackoff is the wait after each failure: 1 m, 10 m, 1 h, 6 h, 24 h,
// then weekly.
func matchBackoff(failures int) time.Duration {
	steps := []time.Duration{time.Minute, 10 * time.Minute, time.Hour, 6 * time.Hour, 24 * time.Hour}
	if failures <= 0 {
		return 0
	}
	if failures <= len(steps) {
		return steps[failures-1]
	}
	return 7 * 24 * time.Hour
}

// bucket is a token bucket.
type bucket struct {
	rate   float64
	burst  float64
	tokens float64
	at     time.Time
	mu     sync.Mutex
}

// take spends one token if there is one.
func (b *bucket) take(now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.at.IsZero() {
		b.tokens, b.at = b.burst, now
	}
	b.tokens = min(b.burst, b.tokens+now.Sub(b.at).Seconds()*b.rate)
	b.at = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

type matchRow struct {
	id                  int64
	key, name, category string
	attempts            int
}

// matchStep runs the due lookups the bucket allows and records each outcome
// in dl_match_queue. It returns how many lookups ran.
func (s *Sync) matchStep(ctx context.Context) int {
	if s.Match == nil || s.DB == nil {
		return 0
	}
	now := s.now()
	rows, err := s.DB.Read().QueryContext(ctx, `SELECT downloader_id, key, name, category, attempts FROM dl_match_queue
		WHERE outcome = '' AND next_try_at <= ? ORDER BY priority, next_try_at LIMIT ?`, now.UTC().Format(store.TimeLayout), matchBurst)
	if err != nil {
		return 0
	}
	var due []matchRow
	for rows.Next() {
		var r matchRow
		if rows.Scan(&r.id, &r.key, &r.name, &r.category, &r.attempts) == nil {
			due = append(due, r)
		}
	}
	rows.Close()
	ran := 0
	for _, r := range due {
		if ctx.Err() != nil || !s.bucket.take(s.now()) {
			break
		}
		ran++
		lctx, cancel := context.WithTimeout(ctx, time.Minute)
		outcome, titleID, err := s.Match(lctx, r.id, r.key, r.name, r.category)
		cancel()
		at := s.now()
		_ = s.DB.Write(context.WithoutCancel(ctx), func(tx *sql.Tx) error {
			if err != nil {
				tries := r.attempts + 1
				out := ""
				if tries >= matchTries {
					out = "nomatch"
				}
				_, e := tx.Exec("UPDATE dl_match_queue SET attempts = ?, next_try_at = ?, last_error = ?, outcome = ? WHERE downloader_id = ? AND key = ?",
					tries, at.Add(matchBackoff(tries)).UTC().Format(store.TimeLayout), firstLine(err.Error()), out, r.id, r.key)
				return e
			}
			var tid any
			if titleID != 0 {
				tid = titleID
			}
			_, e := tx.Exec("UPDATE dl_match_queue SET attempts = attempts + 1, outcome = ?, linked_title_id = ?, last_error = '' WHERE downloader_id = ? AND key = ?",
				outcome, tid, r.id, r.key)
			return e
		})
	}
	return ran
}

func firstLine(s string) string {
	for i, c := range s {
		if c == '\n' {
			return s[:i]
		}
	}
	if len(s) > 300 {
		return s[:300]
	}
	return s
}
