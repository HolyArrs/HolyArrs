package musictrend

import (
	clist "container/list"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Similar artists come from Apple Music's artist view first (when Apple is
// on), then ListenBrainz's keyless similar-artists dataset (listening
// sessions). Apple's artists are matched to MusicBrainz ids: by name among
// ListenBrainz's own similar artists for the same seed, then by kept
// matches, then by a MusicBrainz search made in the background. Answers
// are kept in memory: a week when found, an hour when a source failed,
// knows nothing or some names are still being matched.
const (
	ListenBrainzSimilarURL = "https://labs.api.listenbrainz.org/similar-artists/json"
	similarAlgorithm       = "session_based_days_7500_session_300_contribution_5_threshold_10_limit_100_filter_True_skip_30"
	similarMax             = 25
)

// ErrNoSimilar is returned when the source has no similar artists for an id.
var ErrNoSimilar = errors.New("musictrend: no similar artists")

// ErrPending: Apple Music's similar artists are still being matched to
// MusicBrainz ids; ask again shortly (never kept).
var ErrPending = errors.New("musictrend: similar artists being matched")

// NamedArtist is an artist on another source: its own id and name.
type NamedArtist struct{ ID, Name string }

// SimilarArtist is one artist like the asked one, best first.
type SimilarArtist struct {
	MBID  string `json:"mbid"`
	Name  string `json:"name"`
	Score int64  `json:"score"`
}

// Similar answers similar artists by MusicBrainz id, cached.
type Similar struct {
	URL       string
	UserAgent string
	HTTP      *http.Client
	// TTL keeps found answers (default 7 days); MissTTL failures and empty
	// answers (default 1 hour). Max caps cached ids (default 20000).
	TTL, MissTTL time.Duration
	Max          int
	Now          func() time.Time
	// Report counts source failures (source, code).
	Report func(source, code string)

	// APIURL is the ListenBrainz API base for artist data (default
	// ListenBrainzAPIURL).
	APIURL string
	// Apple answers the seed's similar artists on Apple Music, best first
	// (nil when Apple has none); nil Apple: ListenBrainz only.
	Apple func(ctx context.Context, seed string) ([]NamedArtist, error)
	// Names matches Apple's names to MusicBrainz ids (Service.NameMatcher);
	// NamePace is the wait between two of its searches (default 2 s, so
	// the background matching stays a small share of MusicBrainz; < 0 none).
	Names    *Matcher
	NamePace time.Duration

	mu sync.Mutex
	// db keeps answers across restarts (OpenStore); nil: memory only.
	db      *sql.DB
	stop    chan struct{}
	known   func(mbid string) bool
	writes  int
	entries map[string]similarEntry
	lru     *clist.List               // entry ids, least recently set first
	lruAt   map[string]*clist.Element // each id's place in lru
	flights map[string]*similarFlight
	store   artistStore

	nameMu  sync.Mutex
	nameQ   []string
	queued  map[string]bool
	nameRun bool
}

type similarEntry struct {
	body []byte
	err  error
	at   time.Time
	// short keeps the answer only MissTTL: Apple failed and ListenBrainz
	// answered, or some of Apple's names are still being matched.
	short bool
}

type similarFlight struct {
	done chan struct{}
	e    similarEntry
}

func (s *Similar) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Artists answers the JSON body for one artist and whether it came from
// the cache ("hit") or the source ("miss"). ErrNoSimilar when the source
// knows none; any other error when the source failed.
func (s *Similar) Artists(ctx context.Context, mbid string) ([]byte, string, error) {
	mbid = strings.ToLower(mbid)
	if !validMBID(mbid) {
		return nil, "", ErrNoSimilar
	}
	s.mu.Lock()
	if s.entries == nil {
		s.entries, s.flights = map[string]similarEntry{}, map[string]*similarFlight{}
	}
	old, had := s.entries[mbid]
	if !had {
		old, had = s.loadSimilarLocked(mbid)
	}
	if had && s.now().Sub(old.at) < s.ttl(old) {
		s.mu.Unlock()
		return old.body, "hit", old.err
	}
	if f, ok := s.flights[mbid]; ok {
		s.mu.Unlock()
		select {
		case <-f.done:
			return f.e.body, "hit", f.e.err
		case <-ctx.Done():
			return nil, "", ctx.Err()
		}
	}
	f := &similarFlight{done: make(chan struct{})}
	s.flights[mbid] = f
	s.mu.Unlock()
	return s.fly(ctx, mbid, f, old, had)
}

// fly runs a registered flight: fetches, keeps the answer and ends the
// flight.
func (s *Similar) fly(ctx context.Context, mbid string, f *similarFlight, old similarEntry, had bool) ([]byte, string, error) {
	body, short, err := s.fetch(ctx, mbid)
	f.e = similarEntry{body: body, err: err, at: s.now(), short: short}
	// Stale while revalidate: a failed refetch (budget spent, source down)
	// keeps serving the last good answer and tries again on the next ask.
	quiet := errors.Is(err, ErrNoSimilar) || errors.Is(err, ErrPending)
	if err != nil && !quiet && had && old.err == nil && old.body != nil {
		// Keep it for the failure lifetime, then ask the source again.
		kept := old
		kept.at = s.now().Add(s.ttl(f.e) - s.ttl(old))
		f.e = kept
		s.mu.Lock()
		delete(s.flights, mbid)
		s.setEntryLocked(mbid, kept)
		s.mu.Unlock()
		close(f.done)
		if s.Report != nil {
			s.Report("listenbrainz", "similar_failed")
		}
		return old.body, "stale", nil
	}
	s.mu.Lock()
	delete(s.flights, mbid)
	if ctx.Err() == nil && !errors.Is(err, ErrPending) {
		s.setEntryLocked(mbid, f.e)
		s.keepSimilarLocked(mbid, f.e)
	}
	s.mu.Unlock()
	close(f.done)
	if err != nil && !quiet && s.Report != nil {
		s.Report("listenbrainz", "similar_failed")
	}
	return body, "miss", err
}

func (s *Similar) ttl(e similarEntry) time.Duration {
	if e.err != nil || e.short {
		if s.MissTTL > 0 {
			return s.MissTTL
		}
		return time.Hour
	}
	if s.TTL > 0 {
		return s.TTL
	}
	return 7 * 24 * time.Hour
}

func (s *Similar) max() int {
	if s.Max > 0 {
		return s.Max
	}
	return 20000
}

// setEntryLocked keeps an entry as the most recently set, dropping the
// least recently set one when the cache is full; the caller holds mu.
func (s *Similar) setEntryLocked(mbid string, e similarEntry) {
	if s.entries == nil {
		s.entries, s.flights = map[string]similarEntry{}, map[string]*similarFlight{}
	}
	if s.lru == nil {
		s.lru, s.lruAt = clist.New(), map[string]*clist.Element{}
	}
	if el, ok := s.lruAt[mbid]; ok {
		s.lru.MoveToBack(el)
	} else {
		for len(s.entries) >= s.max() && s.lru.Len() > 0 {
			s.evict()
		}
		s.lruAt[mbid] = s.lru.PushBack(mbid)
	}
	s.entries[mbid] = e
}

// deleteEntryLocked drops one entry; the caller holds mu.
func (s *Similar) deleteEntryLocked(mbid string) {
	delete(s.entries, mbid)
	if el, ok := s.lruAt[mbid]; ok {
		s.lru.Remove(el)
		delete(s.lruAt, mbid)
	}
}

// evict drops the least recently set entry in O(1); the caller holds mu.
func (s *Similar) evict() {
	if s.lru == nil || s.lru.Len() == 0 {
		return
	}
	s.deleteEntryLocked(s.lru.Front().Value.(string))
}

// fetch asks Apple Music, then ListenBrainz; short is true for an answer
// kept only MissTTL.
func (s *Similar) fetch(ctx context.Context, mbid string) (body []byte, short bool, err error) {
	appleFailed := false
	if s.Apple != nil {
		list, err := s.Apple(ctx, mbid)
		switch {
		case err != nil:
			if ctx.Err() != nil {
				return nil, false, ctx.Err()
			}
			appleFailed = true
			if s.Report != nil {
				s.Report("applemusic", "similar_failed")
			}
		case len(list) > 0:
			out, pending := s.mapNames(ctx, mbid, list)
			if len(out) > 0 {
				b, err := json.Marshal(map[string]any{"schema_version": 1, "mbid": mbid, "source": "applemusic", "artists": out})
				return b, pending, err
			}
			if pending {
				return nil, false, ErrPending
			}
		}
	}
	list, err := s.listenBrainz(ctx, mbid)
	if err != nil {
		return nil, false, err
	}
	b, err := json.Marshal(map[string]any{"schema_version": 1, "mbid": mbid, "source": "listenbrainz", "artists": list})
	return b, appleFailed, err
}

// mapNames gives Apple's artists their MusicBrainz ids, in Apple's order
// (score: the list's length down to 1). Names not matched yet are queued
// for a background search; pending says some were.
func (s *Similar) mapNames(ctx context.Context, seed string, list []NamedArtist) (out []SimilarArtist, pending bool) {
	hints := map[string]string{}
	if lb, err := s.listenBrainz(ctx, seed); err == nil {
		for _, a := range lb {
			if k := norm(a.Name); hints[k] == "" {
				hints[k] = a.MBID
			}
		}
	}
	seen := map[string]bool{seed: true}
	out = []SimilarArtist{}
	for i, a := range list {
		id := hints[norm(a.Name)]
		if id == "" && s.Names != nil {
			c, ok, fresh := s.Names.Known(ctx, "artist", a.Name, "")
			switch {
			case ok:
				id = c.MBID
			case !fresh:
				s.queueName(a.Name)
				pending = true
			}
		}
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, SimilarArtist{MBID: id, Name: a.Name, Score: int64(len(list) - i)})
	}
	return out, pending
}

// nameQueueMax caps the names waiting for a search (the rest are queued
// when their seed is asked again).
const nameQueueMax = 5000

// queueName queues one name for a MusicBrainz search; one worker runs the
// queue, so the searches go one at a time at the server's MusicBrainz pace.
func (s *Similar) queueName(name string) {
	if s.Names == nil || s.Names.Search == nil {
		return
	}
	s.nameMu.Lock()
	defer s.nameMu.Unlock()
	if s.queued == nil {
		s.queued = map[string]bool{}
	}
	if s.queued[name] || len(s.nameQ) >= nameQueueMax {
		return
	}
	s.queued[name] = true
	s.nameQ = append(s.nameQ, name)
	if !s.nameRun {
		s.nameRun = true
		go s.matchNames()
	}
}

func (s *Similar) matchNames() {
	for {
		s.nameMu.Lock()
		if len(s.nameQ) == 0 {
			s.nameRun = false
			s.nameMu.Unlock()
			return
		}
		name := s.nameQ[0]
		s.nameQ = s.nameQ[1:]
		s.nameMu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		s.Names.Match(ctx, "artist", name, "")
		cancel()
		s.nameMu.Lock()
		delete(s.queued, name)
		s.nameMu.Unlock()
		switch {
		case s.NamePace == 0:
			time.Sleep(2 * time.Second)
		case s.NamePace > 0:
			time.Sleep(s.NamePace)
		}
	}
}

// listenBrainz reads ListenBrainz's similar artists for one id.
func (s *Similar) listenBrainz(ctx context.Context, mbid string) ([]SimilarArtist, error) {
	base := s.URL
	if base == "" {
		base = ListenBrainzSimilarURL
	}
	client := s.HTTP
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	q := url.Values{"artist_mbids": {mbid}, "algorithm": {similarAlgorithm}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", s.UserAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNoSimilar
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("musictrend: similar artists answered " + resp.Status)
	}
	list, err := ParseLBSimilar(io.LimitReader(resp.Body, 4<<20), mbid)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, ErrNoSimilar
	}
	return list, nil
}

// ParseLBSimilar reads the similar-artists answer: the artists similar to
// ref, best first, without ref itself, bad ids or repeats, at most 25.
func ParseLBSimilar(r io.Reader, ref string) ([]SimilarArtist, error) {
	var rows []struct {
		MBID  string `json:"artist_mbid"`
		Name  string `json:"name"`
		Score int64  `json:"score"`
		Ref   string `json:"reference_mbid"`
	}
	if err := json.NewDecoder(r).Decode(&rows); err != nil {
		return nil, err
	}
	out := []SimilarArtist{}
	seen := map[string]bool{ref: true}
	for _, x := range rows {
		id := strings.ToLower(x.MBID)
		name := strings.TrimSpace(x.Name)
		if !validMBID(id) || name == "" || seen[id] || (x.Ref != "" && strings.ToLower(x.Ref) != ref) {
			continue
		}
		seen[id] = true
		out = append(out, SimilarArtist{MBID: id, Name: name, Score: x.Score})
		if len(out) == similarMax {
			break
		}
	}
	return out, nil
}
