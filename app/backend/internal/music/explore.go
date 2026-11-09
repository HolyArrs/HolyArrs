package music

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
)

// Explore â†’ Music: every row the page shows, built here so they share one
// genre filter and one set of rules. Albums are square, artists round; each
// row says why it is there and hides below its minimum.

// musicRowMin is the fewest cards each music row shows.
var musicRowMin = map[string]int{
	"new_from": 3, "top_albums": 5, "trending_artists": 5,
	"because_have": 4, "fans_of": 4, "missing": 4, "genre": 4, "popular_genre": 4,
}

const (
	// musicGenreTTL keeps an artist's genres; musicSimilarTTL a similar list
	// (misses an hour).
	musicGenreTTL   = 7 * 24 * time.Hour
	musicSimilarTTL = 24 * time.Hour
	// musicGenreLookups caps background MusicBrainz genre lookups an hour.
	musicGenreLookups = 300
	musicCacheMax     = 5000
)

// MusicCard is one album or artist card.
type MusicCard struct {
	Key        string `json:"key"`
	Kind       string `json:"kind"` // album, artist
	MBID       string `json:"mbid"`
	Name       string `json:"name"`
	Artist     string `json:"artist,omitempty"`
	ArtistMBID string `json:"artist_mbid,omitempty"`
	TitleID    int64  `json:"title_id,omitempty"`
	PartID     int64  `json:"part_id,omitempty"`
	Image      string `json:"image,omitempty"`
	// ArtistImage is the artist's picture, on hero candidates: the hero
	// shows it when no candidate album has a cover.
	ArtistImage string   `json:"artist_image,omitempty"`
	Date        string   `json:"date,omitempty"` // YYYY-MM-DD: arrived, out or asked
	State       string   `json:"state,omitempty"`
	Rank        int      `json:"rank,omitempty"`
	Listens     int64    `json:"listen_count,omitempty"`
	Upcoming    bool     `json:"upcoming,omitempty"`
	Genres      []string `json:"genres"`
	// Note is the card's own reason line, when the row gives one.
	Note string `json:"note,omitempty"`
	// Fresh is the card's freshness strip (Recently Added, New Release).
	Fresh *engine.Fresh `json:"fresh,omitempty"`
	// Badge names a new release (New album, New EP, Album coming).
	Badge string `json:"badge,omitempty"`
	// Kicker is a Spotlight card's label above its title (New album, Climbing).
	Kicker string `json:"kicker,omitempty"`
	// Following: the member waits for this album (New & Popular, Coming soon).
	Following bool `json:"following,omitempty"`
	// Explicit: the release has explicit lyrics (the "E" mark; Deezer says so).
	Explicit bool `json:"explicit,omitempty"`
	// Track names a song on a Fresh tracks line; the card asks for its album.
	Track string `json:"track,omitempty"`
	// Hue is a genre tile's colour index.
	Hue *int `json:"hue,omitempty"`
	// Images is a genre tile's slideshow of artist photos (Image is the first).
	Images []string `json:"images,omitempty"`
	// Cover is a Browse tile's tilted album cover (music_lists_v1): the
	// genre's top charted album not used by another tile.
	Cover string `json:"cover,omitempty"`
	// Href is a Browse tile's page when it is not a genre page (row
	// templates: moods, seasons, decades).
	Href string `json:"href,omitempty"`
}

// MusicRow is one row of Explore â†’ Music.
type MusicRow struct {
	ID     string      `json:"id"`
	Type   string      `json:"type"`
	Title  string      `json:"title"`
	Reason string      `json:"reason"`
	Shape  string      `json:"shape"` // square, round
	Items  []MusicCard `json:"items"`
}

type musicGenreEntry struct {
	genres []string
	links  []metadata.ArtistLink
	at     time.Time
}

type musicSimilarEntry struct {
	list []metadata.SimilarArtist
	ok   bool
	// pending: the metadata server was still getting the list (202); it
	// is asked again after similarPendingAgain, not kept as a miss.
	pending bool
	at      time.Time
	done    chan struct{}
}

// similarPendingAgain is how long a "still getting it" answer is kept
// before the server is asked again.
var similarPendingAgain = 5 * time.Second

// similarPending reports a similar list the server is still getting (or
// one being asked right now).
func (h *Handlers) similarPending(id string) bool {
	c := &h.musicx
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.similar[id]
	return e != nil && (e.done != nil || e.pending)
}

// musicExplore keeps artists' genres and similar lists between requests.
type musicExplore struct {
	mu      sync.Mutex
	genres  map[string]musicGenreEntry
	queue   []string
	queued  map[string]bool
	running bool
	lookups []time.Time
	similar map[string]*musicSimilarEntry
}

type genreSource interface {
	ArtistGenres(ctx context.Context, mbid string) ([]string, error)
}

// genresOf answers the known genres of each id and queues the rest for a
// background MusicBrainz lookup.
func (h *Handlers) genresOf(ids []string) map[string][]string {
	c := &h.musicx
	out := map[string][]string{}
	src, _ := h.genreProvider()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.genres == nil {
		c.genres, c.queued = map[string]musicGenreEntry{}, map[string]bool{}
	}
	for _, id := range ids {
		if id == "" {
			continue
		}
		e, ok := c.genres[id]
		if ok {
			out[id] = e.genres
		}
		if (!ok || time.Since(e.at) > musicGenreTTL) && src != nil && !c.queued[id] && len(c.queue) < 400 {
			c.queue = append(c.queue, id)
			c.queued[id] = true
		}
	}
	if src != nil && len(c.queue) > 0 && !c.running {
		c.running = true
		go h.fillGenres(src)
	}
	return out
}

func (h *Handlers) genreProvider() (genreSource, bool) {
	if h.lib() == nil || h.lib().Artists == nil {
		return nil, false
	}
	g, ok := h.lib().Artists.(genreSource)
	return g, ok
}

// fillGenres looks the queued artists up one at a time (MusicBrainz's own
// limiter spaces them), at most musicGenreLookups an hour.
func (h *Handlers) fillGenres(src genreSource) {
	c := &h.musicx
	for {
		c.mu.Lock()
		now := time.Now()
		kept := c.lookups[:0]
		for _, t := range c.lookups {
			if now.Sub(t) < time.Hour {
				kept = append(kept, t)
			}
		}
		c.lookups = kept
		if len(c.queue) == 0 || len(c.lookups) >= musicGenreLookups {
			c.running = false
			c.mu.Unlock()
			return
		}
		id := c.queue[0]
		c.queue = c.queue[1:]
		c.lookups = append(c.lookups, now)
		c.mu.Unlock()
		// A background fill: paced and paused by the metadata server gate.
		ctx, cancel := context.WithTimeout(metadata.Ahead(context.Background()), time.Minute)
		var g []string
		var links []metadata.ArtistLink
		var err error
		if gl, ok := src.(genreLinkSource); ok {
			g, links, err = gl.ArtistGenresLinks(ctx, id)
		} else {
			g, err = src.ArtistGenres(ctx, id)
		}
		cancel()
		c.mu.Lock()
		delete(c.queued, id)
		if err == nil {
			if len(c.genres) >= musicCacheMax {
				c.genres = map[string]musicGenreEntry{}
			}
			c.genres[id] = musicGenreEntry{genres: g, links: links, at: time.Now()}
		}
		c.mu.Unlock()
	}
}

// similarTo reads the similar artists of each seed, waiting at most wait
// for answers not cached yet (they keep filling in the background).
func (h *Handlers) similarTo(seeds []string, wait time.Duration) map[string][]metadata.SimilarArtist {
	out := map[string][]metadata.SimilarArtist{}
	if h.MusicSimilar == nil {
		return out
	}
	c := &h.musicx
	// pending holds the done channels, taken under the lock: fetchSimilar
	// clears e.done under the lock, so reading it later would race.
	var pending []chan struct{}
	c.mu.Lock()
	if c.similar == nil {
		c.similar = map[string]*musicSimilarEntry{}
	}
	entries := map[string]*musicSimilarEntry{}
	for _, id := range seeds {
		e := c.similar[id]
		fresh := e != nil && (e.done == nil) && ((e.ok && time.Since(e.at) < musicSimilarTTL) || (!e.ok && !e.pending && time.Since(e.at) < time.Hour) || (e.pending && time.Since(e.at) < similarPendingAgain))
		if e == nil || (!fresh && e.done == nil) {
			if len(c.similar) >= musicCacheMax {
				c.similar = map[string]*musicSimilarEntry{}
			}
			e = &musicSimilarEntry{done: make(chan struct{})}
			c.similar[id] = e
			go h.fetchSimilar(id, e)
		}
		if e.done != nil {
			pending = append(pending, e.done)
		}
		entries[id] = e
	}
	c.mu.Unlock()
	// One deadline for all: a timer's channel fires once, so after it the
	// loop would wait on every other pending seed in full.
	timer := time.NewTimer(wait)
	defer timer.Stop()
wait:
	for _, done := range pending {
		select {
		case <-done:
		case <-timer.C:
			break wait
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, e := range entries {
		if e.done == nil && e.ok {
			out[id] = e.list
		}
	}
	return out
}

func (h *Handlers) fetchSimilar(id string, e *musicSimilarEntry) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	list, err := h.MusicSimilar(ctx, id)
	c := &h.musicx
	c.mu.Lock()
	e.list, e.ok, e.at = list, err == nil, time.Now()
	e.pending = errors.Is(err, metadata.ErrServicePending)
	done := e.done
	e.done = nil
	c.mu.Unlock()
	close(done)
}

// musicArtist is one artist in the Library.
type musicArtist struct {
	id     int64
	name   string
	mbid   string
	image  string
	files  int
	added  time.Time
	genres []string
}

// artistImage is the picture link of a library artist: the picture route
// (the metadata server's sharp photo first) when the artist has a
// MusicBrainz id, with the saved library artwork as the route's fallback;
// without an id, the saved artwork alone.
func artistImage(t mediafiles.Title) string {
	saved := ""
	if l := artworkLinks(t.Artwork); len(l) > 0 && strings.HasPrefix(l[0], "/api/v1/artwork/") {
		saved = l[0]
	}
	mbid := strings.ToLower(t.SourceIDs["musicbrainz"])
	if !metadata.ValidMBID(mbid) {
		return saved
	}
	if saved == "" {
		return musicArtPrefix + "artist/" + mbid
	}
	return musicArtPrefix + "artist/" + mbid + "?fallback=" + url.QueryEscape(saved)
}

func dayOf(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format("2006-01-02")
}
