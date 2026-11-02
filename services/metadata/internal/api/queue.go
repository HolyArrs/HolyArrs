package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/fetchq"
	"github.com/CoderLuii/holyarrs-metadata/internal/model"
)

// FetchQueue is the fetch-ahead queue (internal/fetchq).
type FetchQueue interface {
	Enqueue(ctx context.Context, key string, p int, wait bool) <-chan error
}

// positioner is a queue that can tell a job's place in line.
type positioner interface {
	Position(ctx context.Context, key string) (string, int)
}

// Page text for an item still on its way (AC 9): never "not found".
const (
	msgFetching = "Getting this from the server…"
	msgQueued   = "Queued, number %d in line"
)

// withQueueState adds where the job for key stands to a 202 body: state
// "queued" with its position and the page's text, else "fetching".
func (s *Server) withQueueState(ctx context.Context, key string, body map[string]any) map[string]any {
	body["state"], body["position"], body["message"] = "fetching", 0, msgFetching
	if pq, ok := s.opts.Queue.(positioner); ok {
		if st, n := pq.Position(ctx, key); st == "queued" && n > 0 {
			body["state"], body["position"], body["message"] = "queued", n, fmt.Sprintf(msgQueued, n)
		}
	}
	return body
}

// QueueKey is the job key of a MusicBrainz detail answer.
func QueueKey(kind, mbid string) string { return "mb:" + kind + ":" + strings.ToLower(mbid) }

func queueQuery(key string) (model.Query, bool) {
	rest, ok := strings.CutPrefix(key, "mb:")
	if !ok {
		return model.Query{}, false
	}
	kind, id, ok := strings.Cut(rest, ":")
	if !ok {
		return model.Query{}, false
	}
	q, err := parseQuery(&url.URL{Path: "/v1/music/musicbrainz/" + kind + "/" + id})
	if err != nil || q.ID == "" {
		return model.Query{}, false
	}
	return q, err == nil
}

// serveQueued answers a MusicBrainz detail miss through the queue: a P0
// job (P4 for prefetch marked X-Fetch: ahead, which never waits), up to
// QueueWait for it, then a partial answer with Retry-After.
func (s *Server) serveQueued(w http.ResponseWriter, r *http.Request, q model.Query) {
	ahead := r.Header.Get("X-Fetch") == "ahead"
	p := fetchq.P0Waiting
	if ahead {
		p = fetchq.P4Deep
	}
	// The install's P0 asks share the lane fairly with other installs'.
	who, _ := r.Context().Value(callerKey{}).(caller)
	done := s.opts.Queue.Enqueue(fetchq.WithInstall(context.Background(), who.who), QueueKey(q.Kind, q.ID), p, !ahead)
	if done != nil {
		wait := s.opts.QueueWait
		if wait <= 0 {
			wait = 800 * time.Millisecond
		}
		select {
		case err := <-done:
			if err == nil {
				if e, found, gerr := s.store.Get(r.Context(), q.Key()); gerr == nil && found {
					s.misses.Add(1)
					s.respond(w, q, e, false, "miss")
					return
				}
			} else if errors.Is(err, fetchq.ErrNotFound) {
				problem(w, 404, "upstream_status", 0)
				return
			}
		case <-time.After(wait):
		case <-r.Context().Done():
		}
	}
	w.Header().Set("Retry-After", "2")
	w.Header().Set("X-Metadata-Cache", "queued")
	writeJSON(w, 202, s.withQueueState(r.Context(), QueueKey(q.Kind, q.ID), map[string]any{"schema_version": 1, "source": q.Provider, "source_id": q.ID, "stale": false, "completeness": "partial", "partial": true, "data": nil}))
}

// RunJob is the queue's worker for MusicBrainz detail jobs: it fetches only
// when no answer is kept, the kept one expired, or the change feed marked
// it stale (never re-fetching unchanged data), then calls OnFetched.
func (s *Server) RunJob(ctx context.Context, key string, _ int) error {
	q, ok := queueQuery(key)
	if !ok {
		return fetchq.ErrNotFound
	}
	if _, ok := s.slim(ctx, q); ok {
		// The slim copy answers it: no live call.
		if s.opts.OnFetched != nil {
			s.opts.OnFetched(q.Kind, q.ID)
		}
		return nil
	}
	cached, found, err := s.store.Get(ctx, q.Key())
	if err != nil {
		return err
	}
	if found && time.Now().Before(cached.ExpiresAt) {
		fresh := true
		if s.opts.Refresh != nil {
			_, stale, _ := s.opts.Refresh.State(ctx, cached.Key)
			fresh = !stale
		}
		if fresh {
			return nil
		}
	}
	if _, _, err = s.load(ctx, q, found); err != nil {
		var fe *model.FetchError
		if errors.As(err, &fe) {
			switch {
			case fe.Status == 404 || fe.Status == 400:
				return fetchq.ErrNotFound
			case fe.Status == 503 || fe.Status == 429 || fe.Code == "upstream_budget":
				return fetchq.ErrThrottled
			}
		}
		return err
	}
	if s.opts.OnFetched != nil {
		s.opts.OnFetched(q.Kind, q.ID)
	}
	return nil
}

// SetQueue attaches the queue (made after the server, since the queue's
// worker is the server's RunJob). Call before serving.
func (s *Server) SetQueue(q FetchQueue) { s.opts.Queue = q }

// StoredData answers the stored MusicBrainz answer of an item, if any.
func (s *Server) StoredData(ctx context.Context, kind, mbid string) ([]byte, bool) {
	q, ok := queueQuery(QueueKey(kind, mbid))
	if !ok {
		return nil, false
	}
	if e, ok := s.slim(ctx, q); ok {
		return e.Result.Data, true
	}
	e, found, err := s.store.Get(ctx, q.Key())
	if err != nil || !found {
		return nil, false
	}
	return e.Result.Data, true
}

// slimTTL is how long a slim-copy answer counts as fresh in a response
// (the copy is renewed by the dump refresh, not by expiry).
const slimTTL = 7 * 24 * time.Hour

// slim answers a MusicBrainz detail lookup from the slim dump copy.
func (s *Server) slim(ctx context.Context, q model.Query) (model.Entry, bool) {
	if s.opts.Slim == nil || q.Provider != "musicbrainz" || q.Operation != "detail" || q.ID == "" {
		return model.Entry{}, false
	}
	b, date, ok := s.opts.Slim(ctx, q.Kind, q.ID)
	if !ok {
		return model.Entry{}, false
	}
	at, err := time.Parse("20060102-150405", date)
	if err != nil {
		at = time.Now()
	}
	now := time.Now().UTC()
	return model.Entry{Key: q.Key(), Result: model.Result{Data: b, Complete: true}, FetchedAt: at.UTC(), ExpiresAt: now.Add(slimTTL)}, true
}

// MBStoreKey is the store key of a MusicBrainz detail answer.
func MBStoreKey(kind, mbid string) string {
	q, ok := queueQuery(QueueKey(kind, mbid))
	if !ok {
		return ""
	}
	return q.Key()
}
