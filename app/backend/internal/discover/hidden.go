package discover

import (
	"context"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Hide keeps one title, or one genre of a kind, out of browsing for the
// household (UserID 0) or for one person. It is a browsing choice, not a
// block: links, the Library and asks keep working, and age limits and
// Never add stay the strict tools.
type Hide struct {
	ID     int64  `json:"id"`
	Kind   string `json:"kind"`
	TMDBID string `json:"tmdb_id,omitempty"`
	// Genre is the English TMDB genre name for Kind.
	Genre string `json:"genre,omitempty"`
	// Name is the title's name when it was hidden ("" for a genre).
	Name      string `json:"name,omitempty"`
	UserID    int64  `json:"user_id"`
	CreatedAt string `json:"created_at"`
}

// hideRules are the hides one person browses under.
type hideRules struct {
	titles map[[2]string]bool // kind, TMDB id
	genres map[[2]string]bool // kind, genre
}

func (r hideRules) empty() bool { return len(r.titles) == 0 && len(r.genres) == 0 }

// hides reports whether the rules keep the title out of browsing.
func (r hideRules) hides(kind, tmdbID string, genres []string) bool {
	if r.titles[[2]string{kind, tmdbID}] {
		return true
	}
	for _, g := range genres {
		if r.genres[[2]string{kind, g}] {
			return true
		}
	}
	return false
}

func (r *hideRules) add(x Hide) {
	if r.titles == nil {
		r.titles, r.genres = map[[2]string]bool{}, map[[2]string]bool{}
	}
	if x.Genre != "" {
		r.genres[[2]string{x.Kind, x.Genre}] = true
	} else {
		r.titles[[2]string{x.Kind, x.TMDBID}] = true
	}
}

// hiddenSet is every hide, ready to check.
type hiddenSet struct {
	rows      []Hide
	household hideRules
	person    map[int64]hideRules
}

func newHiddenSet(rows []Hide) *hiddenSet {
	s := &hiddenSet{rows: rows, person: map[int64]hideRules{}}
	for _, x := range rows {
		if x.UserID == 0 {
			s.household.add(x)
			continue
		}
		p := s.person[x.UserID]
		p.add(x)
		s.person[x.UserID] = p
	}
	return s
}

// For is the household's hides plus the person's own.
func (s *hiddenSet) For(user int64) hideRules {
	own, ok := s.person[user]
	if !ok || own.empty() {
		return s.household
	}
	if s.household.empty() {
		return own
	}
	var both hideRules
	for _, x := range s.rows {
		if x.UserID == 0 || x.UserID == user {
			both.add(x)
		}
	}
	return both
}

// hiddenReload is how long the kept hides are trusted before reading them
// again; every change through HolyArrs drops them at once.
const hiddenReload = 10 * time.Minute

type hiddenKept struct {
	mu  sync.Mutex
	at  time.Time
	set *hiddenSet
	gen uint64
}

// hiddenByStore keeps each database's hides in memory, so browsing with no
// hides (or the same ones) costs no query per request.
var hiddenByStore sync.Map // *database.Store -> *hiddenKept

func keptHidden(s *database.Store) *hiddenKept {
	v, _ := hiddenByStore.LoadOrStore(s, &hiddenKept{})
	return v.(*hiddenKept)
}

// hiddenFor is s's hides, read once and kept.
func hiddenFor(ctx context.Context, s *database.Store) (*hiddenSet, error) {
	k := keptHidden(s)
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.set != nil && time.Since(k.at) < hiddenReload {
		return k.set, nil
	}
	rows, err := readHides(ctx, s.Read())
	if err != nil {
		return nil, err
	}
	k.set, k.at = newHiddenSet(rows), time.Now()
	return k.set, nil
}

// forgetHidden drops s's kept hides after a change, and moves the
// generation on so answers built with the old ones are not reused.
func forgetHidden(s *database.Store) {
	k := keptHidden(s)
	k.mu.Lock()
	k.set = nil
	k.gen++
	k.mu.Unlock()
}

// hiddenGen changes every time s's hides change.
func hiddenGen(s *database.Store) uint64 {
	k := keptHidden(s)
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.gen
}

// hideRulesFor are the rules for whoever is signed in on ctx: the
// household's hides, plus the person's own. With no one signed in (a
// background job) only the household's apply.
func hideRulesFor(ctx context.Context, s *database.Store) (hideRules, error) {
	set, err := hiddenFor(ctx, s)
	if err != nil {
		return hideRules{}, err
	}
	who, ok := webserver.From(ctx)
	if !ok {
		return set.household, nil
	}
	return set.For(who.UserID), nil
}

// mayFindHidden: the person may still find a hidden title to add it
// (managers), where everyone else just does not see it.
func mayFindHidden(ctx context.Context) bool {
	who, ok := webserver.From(ctx)
	return !ok || who.Has(webserver.PermManageLibrary)
}

// dropHidden leaves out the items ctx's person has hidden.
func (h *Handlers) dropHidden(ctx context.Context, items []metadata.DiscoverItem) ([]metadata.DiscoverItem, error) {
	rules, err := hideRulesFor(ctx, h.Store)
	if err != nil || rules.empty() {
		return items, err
	}
	out := make([]metadata.DiscoverItem, 0, len(items))
	for _, it := range items {
		if !rules.hides(it.Kind, it.TMDBID, it.Genres) {
			out = append(out, it)
		}
	}
	return out, nil
}

// HiddenMark says how a looked-up title shows to the person on ctx:
// hidden is true when their hides cover it; drop is true when
// they should not see it at all (not a manager, who still sees it, marked).
func HiddenMark(ctx context.Context, s *database.Store, kind, tmdbID string, genres []string) (hidden, drop bool) {
	rules, err := hideRulesFor(ctx, s)
	if err != nil || rules.empty() || !rules.hides(kind, tmdbID, genres) {
		return false, false
	}
	return true, !mayFindHidden(ctx)
}

// hidesOf are the stored hides that cover the title (its own and its
// genres'), for every person: what a manager's title page can undo.
func hidesOf(set *hiddenSet, kind, tmdbID string, genres []string) []Hide {
	out := []Hide{}
	for _, x := range set.rows {
		if x.Kind != kind {
			continue
		}
		if (x.TMDBID != "" && x.TMDBID == tmdbID) || (x.Genre != "" && slices.Contains(genres, x.Genre)) {
			out = append(out, x)
		}
	}
	return out
}

// titleHides are what a title page says about hides: whether the viewer's
// hides cover it, and for owners and admins every hide that does.
func (h *Handlers) titleHides(r *http.Request, it metadata.DiscoverItem) (bool, []Hide, error) {
	set, err := hiddenFor(r.Context(), h.Store)
	if err != nil || len(set.rows) == 0 {
		return false, nil, err
	}
	rules, err := hideRulesFor(r.Context(), h.Store)
	if err != nil {
		return false, nil, err
	}
	hidden := rules.hides(it.Kind, it.TMDBID, it.Genres)
	// Whose hides they are needs what the hide list needs: an owner or
	// admin, and from an API key the admin scope.
	if who, ok := webserver.From(r.Context()); ok && !(who.Has(webserver.NeedUsers) && who.HasScope(webserver.ScopeAdmin)) {
		return hidden, nil, nil
	}
	return hidden, hidesOf(set, it.Kind, it.TMDBID, it.Genres), nil
}

// dropHiddenMovies leaves out the movies ctx's person has hidden.
func (h *Handlers) dropHiddenMovies(ctx context.Context, ms []metadata.Movie) []metadata.Movie {
	rules, err := hideRulesFor(ctx, h.Store)
	if err != nil || rules.empty() {
		return ms
	}
	out := make([]metadata.Movie, 0, len(ms)) // ms may be a kept list: never trim it in place
	for _, m := range ms {
		if !rules.hides("movie", m.IDs["tmdb"], m.Genres) {
			out = append(out, m)
		}
	}
	return out
}

// dropHiddenCredits leaves out the credits whose movie ctx's person has
// hidden (a people page).
func (h *Handlers) dropHiddenCredits(ctx context.Context, cs []metadata.PersonMovieCredit) []metadata.PersonMovieCredit {
	rules, err := hideRulesFor(ctx, h.Store)
	if err != nil || rules.empty() {
		return cs
	}
	out := make([]metadata.PersonMovieCredit, 0, len(cs))
	for _, c := range cs {
		if !rules.hides("movie", c.Movie.IDs["tmdb"], c.Movie.Genres) {
			out = append(out, c)
		}
	}
	return out
}
