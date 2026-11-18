package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// BatchTMDBPath is the metadata server's route for many TMDB reads in one
// request. Servers before it answer the route with an error; the reads then
// go one by one as before.
const BatchTMDBPath = "/v1/batch/tmdb"

// Batch limits: the server takes at most 40 reads a request, and a request
// line of at most 4096 bytes.
const (
	batchMaxReads = 40
	batchMaxQuery = 3500
	// batchOffFor is how long an install waits before trying the batch route
	// again after the server lacked it (a server update is picked up).
	batchOffFor = time.Hour
)

// TMDBRead is one TMDB read (path and query), as get sends it.
type TMDBRead struct {
	Path  string
	Query url.Values
}

// MovieSearchRead is the read Search makes for query and year.
func MovieSearchRead(query string, year int) TMDBRead {
	q := url.Values{"query": {query}, "include_adult": {"false"}}
	if year > 0 {
		q.Set("year", strconv.Itoa(year))
	}
	return TMDBRead{Path: "/search/movie", Query: q}
}

// MovieDetailRead is the read Movie makes for "tmdb:<id>".
func MovieDetailRead(id int64) TMDBRead {
	return TMDBRead{Path: "/movie/" + strconv.FormatInt(id, 10), Query: url.Values{"append_to_response": {"release_dates,images,credits,translations,alternative_titles"}, "include_image_language": {"en,null"}}}
}

func (r TMDBRead) suffix() string {
	if len(r.Query) == 0 {
		return r.Path
	}
	return r.Path + "?" + r.Query.Encode()
}

func (t *TMDB) batchOff(now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return now.Before(t.batchOffUntil)
}

// Prefetch asks the metadata server for many reads in few requests and keeps
// the answers as get would, so the reads that follow are answered here. It
// returns how many reads it kept. Reads it could not get (refused, failed,
// own key, an older server) are left alone: get asks for them one by one.
func (t *TMDB) Prefetch(ctx context.Context, reads []TMDBRead) int {
	r := t.route(ctx)
	base := r.base()
	server, ok := strings.CutSuffix(base, TMDBPath)
	if r.Off || !ok || r.Access == nil || noCache(ctx) || t.batchOff(time.Now()) {
		return 0
	}
	var todo []TMDBRead
	seen := map[string]bool{}
	for _, rd := range reads {
		u := base + rd.suffix()
		if seen[u] {
			continue
		}
		seen[u] = true
		if c, ok := t.kept(ctx, u, t.Disk != nil && !strings.HasPrefix(rd.Path, "/search")); ok && time.Since(c.at) < t.ttl() {
			continue
		}
		todo = append(todo, rd)
	}
	kept := 0
	for len(todo) > 0 && ctx.Err() == nil {
		n, size := 0, 0
		for n < len(todo) && n < batchMaxReads {
			l := len(url.QueryEscape(strings.TrimPrefix(todo[n].suffix(), "/"))) + 3
			if n > 0 && size+l > batchMaxQuery {
				break
			}
			size += l
			n++
		}
		got, more := t.batch(ctx, r, server, base, todo[:n])
		kept += got
		if !more {
			break
		}
		todo = todo[n:]
	}
	return kept
}

func (t *TMDB) ttl() time.Duration {
	if t.TTL > 0 {
		return t.TTL
	}
	return 6 * time.Hour
}

// batch sends one batch request; more is false when the rest should not be
// tried now (refused, failing, or no batch route).
func (t *TMDB) batch(ctx context.Context, r TMDBRoute, server, base string, reads []TMDBRead) (kept int, more bool) {
	tok, err := r.Access(ctx)
	if err != nil || tok == "" {
		return 0, false
	}
	v := url.Values{}
	for _, rd := range reads {
		v.Add("r", strings.TrimPrefix(rd.suffix(), "/"))
	}
	if err := t.wait(ctx); err != nil {
		return 0, false
	}
	rctx, cancel := afterGate(ctx, 2*tmdbAttempt)
	defer cancel()
	req, _ := http.NewRequestWithContext(rctx, http.MethodGet, server+BatchTMDBPath+"?"+v.Encode(), nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	aheadRequest(ctx, req)
	cl := r.HTTP
	if cl == nil {
		cl = http.DefaultClient
	}
	resp, err := cl.Do(req)
	if err != nil {
		return 0, false
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return 0, false
	case resp.StatusCode != http.StatusOK:
		// A server without the route: one by one, and look again later.
		t.mu.Lock()
		t.batchOffUntil = time.Now().Add(batchOffFor)
		t.mu.Unlock()
		return 0, false
	}
	var out struct {
		Items []struct {
			Status int             `json:"status"`
			Body   json.RawMessage `json:"body"`
		} `json:"items"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(&out); err != nil || len(out.Items) != len(reads) {
		return 0, !errors.Is(err, context.Canceled)
	}
	now := time.Now()
	for i, it := range out.Items {
		u := base + reads[i].suffix()
		disk := t.Disk != nil && !strings.HasPrefix(reads[i].Path, "/search")
		switch {
		case it.Status == http.StatusOK && len(it.Body) > 0:
			t.keep(ctx, u, cached{body: it.Body, at: now}, disk)
			kept++
		case it.Status == http.StatusNotFound:
			t.keep(ctx, u, cached{at: now, miss: true}, disk)
			kept++
		}
	}
	return kept, true
}
