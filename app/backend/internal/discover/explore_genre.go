package discover

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
)

// ExploreGenreParam reads ?genre= for kind: the TMDB genre id and name.
func ExploreGenreParam(v, kind string) (int, string, bool) {
	if v == "" {
		return 0, "", true
	}
	id, err := strconv.Atoi(v)
	if err != nil || engine.GenreName(kind, id) == "" {
		return 0, "", false
	}
	return id, engine.GenreName(kind, id), true
}

// Kinds0 is the first of kinds, else the tab's first kind.
func Kinds0(kinds []string, sw string) string {
	if len(kinds) > 0 {
		return kinds[0]
	}
	return engine.Kinds(sw)[0]
}

// MergeGenreRows folds the show page's rows into the movie page's: a row
// both have takes turns between the two kinds; the rest follow. The
// "Because you have" row keeps the first seed's.
func MergeGenreRows(movie, show []engine.Row) []engine.Row {
	out := append([]engine.Row{}, movie...)
	at := map[string]int{}
	for i, r := range out {
		at[r.ID] = i
	}
	for _, r := range show {
		i, ok := at[r.ID]
		switch {
		case !ok:
			at[r.ID] = len(out)
			out = append(out, r)
		case r.ID != "genre_because":
			out[i].Items = engine.Interleave([][]engine.Item{out[i].Items, r.Items}, engine.RowMax)
		}
	}
	return out
}

// genreRowsTTL is how long a genre page's rows are kept in memory.
const genreRowsTTL = 15 * time.Minute

// GenreRowCache keeps built genre-page rows (before per-person views) so a
// second visit skips the outside lists.
type GenreRowCache struct {
	mu   sync.Mutex
	rows map[string]genreRowsEntry
}

type genreRowsEntry struct {
	at   time.Time
	rows []engine.Row
}

// Get returns a copy of the rows kept for key.
func (c *GenreRowCache) Get(key string) ([]engine.Row, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.rows[key]
	if !ok || time.Since(e.at) > genreRowsTTL {
		return nil, false
	}
	out := make([]engine.Row, len(e.rows))
	copy(out, e.rows)
	return out, true
}

// Put keeps a copy of rows for key.
func (c *GenreRowCache) Put(key string, rows []engine.Row) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.rows == nil || len(c.rows) > 200 {
		c.rows = map[string]genreRowsEntry{}
	}
	keep := make([]engine.Row, len(rows))
	copy(keep, rows)
	c.rows[key] = genreRowsEntry{at: time.Now(), rows: keep}
}

// GenreRequests are a genre page's shared lists, in row order: trending,
// the genre's popular titles, best new, classics, then one per mood (the
// "similar" list for the seed comes last when there is one).
func GenreRequests(kind, genre string, now time.Time) []engine.Request {
	reqs := []engine.Request{
		{List: "trending"},
		{List: "discover", Genre: genre, MinVotes: 50},
		{List: "discover", Genre: genre, From: now.AddDate(-1, 0, 0).Format("2006-01-02"), To: now.Format("2006-01-02"), MinVote: 6.5, MinVotes: 50},
		{List: "discover", Genre: genre, To: now.AddDate(-20, 0, 0).Format("2006-01-02"), MinVotes: 1000, Sort: "rating"},
	}
	for _, m := range metadata.SubMoods(genre) {
		reqs = append(reqs, engine.Request{List: "discover", Genre: genre, Keywords: strings.Join(m.Keywords, "|"), MinVotes: 20})
	}
	for i := range reqs {
		reqs[i].Kind, reqs[i].Scope = kind, engine.ScopeShared
	}
	return reqs
}
