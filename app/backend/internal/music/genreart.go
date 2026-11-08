package music

import (
	"context"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"sync"
	"time"
)

// SlugRe is a music genre's URL name: lowercase letters, digits and dashes.
var SlugRe = regexp.MustCompile(`^[a-z0-9-]{1,40}$`)

// GenreArtMax caps the genre tile pictures kept in memory.
const GenreArtMax = 200

// GenreArtCache keeps genre tile pictures by slug and version.
type GenreArtCache struct {
	mu   sync.Mutex
	pics map[string]genrePic
}

type genrePic struct {
	b  []byte
	at time.Time
}

// Len is the number of pictures kept.
func (c *GenreArtCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.pics)
}

// GenreArtHandlers serves the grey genre tile pictures.
type GenreArtHandlers struct {
	// Tile reads a tile picture from the metadata server. Nil answers 404.
	Tile func(ctx context.Context, slug, version string) ([]byte, error)
	// KidMax is the Kid's age cap for the request, "" for adults.
	KidMax func(r *http.Request) string
	Cache  *GenreArtCache
}

// Routes registers GET /api/v1/music/genre-art/{slug}.
func (h GenreArtHandlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/music/genre-art/{slug}", h.Art)
}

// Art answers GET /api/v1/music/genre-art/{slug}?v=: the grey tile picture
// from the metadata server, kept in memory.
func (h GenreArtHandlers) Art(w http.ResponseWriter, r *http.Request) {
	slug, v := r.PathValue("slug"), r.URL.Query().Get("v")
	n, _ := strconv.Atoi(r.URL.Query().Get("n"))
	if n < 0 || n > 3 {
		http.NotFound(w, r)
		return
	}
	if h.KidMax(r) != "" || !SlugRe.MatchString(slug) || len(v) > 32 || h.Tile == nil {
		http.NotFound(w, r)
		return
	}
	// Photo n of the slideshow is "<slug>.<n>" on the metadata server.
	name := slug
	if n > 0 {
		name += "." + strconv.Itoa(n)
	}
	key := name + "?" + v
	c := h.Cache
	c.mu.Lock()
	got, ok := c.pics[key]
	c.mu.Unlock()
	pic := got.b
	if !ok {
		fetched, err := h.Tile(r.Context(), name, v)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		pic = fetched
		c.mu.Lock()
		if c.pics == nil {
			c.pics = map[string]genrePic{}
		}
		keepUnder(c.pics, GenreArtMax, func(p genrePic) time.Time { return p.at })
		c.pics[key] = genrePic{b: pic, at: time.Now()}
		c.mu.Unlock()
	}
	w.Header().Set("Content-Type", "image/jpeg")
	if v != "" {
		// v is the picture's content version: a new picture gets a new URL.
		w.Header().Set("Cache-Control", "private, max-age=604800, immutable")
	} else {
		w.Header().Set("Cache-Control", "private, max-age=3600")
	}
	_, _ = w.Write(pic)
}

// keepUnder makes room for one more entry in m under limit by dropping the
// oldest tenth (by at), so hitting a cap never empties a cache at once.
func keepUnder[K comparable, V any](m map[K]V, limit int, at func(V) time.Time) {
	if len(m) < limit {
		return
	}
	times := make([]time.Time, 0, len(m))
	for _, v := range m {
		times = append(times, at(v))
	}
	slices.SortFunc(times, func(a, b time.Time) int { return a.Compare(b) })
	drop := max(len(m)-limit+1, limit/10)
	cut := times[min(drop, len(times))-1]
	for k, v := range m {
		if !at(v).After(cut) && drop > 0 {
			delete(m, k)
			drop--
		}
	}
}
