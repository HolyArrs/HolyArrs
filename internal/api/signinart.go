package api

import (
	"cmp"
	"context"
	"math/rand/v2"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/ratings"
)

// The sign-in page's art: one wide picture at a time behind the card. The
// route is public (nobody is signed in yet), so the answer holds picture
// addresses and sizes only: no names, ids, counts or anything else.
//
// The pictures come from the Library (when the owner leaves "Show Library
// art on the sign-in page" on) and fall back to trending titles. Either way
// only kid-safe titles are used: movies up to PG-13, shows up to TV-PG, and
// never a title without a rating.

const (
	// signinArtMin is how many Library titles must qualify before the
	// Library is used instead of trending titles.
	signinArtMin = 6
	// signinArtMax is how many pictures one answer holds.
	signinArtMax = 12
	// signinArtLookups caps the image list calls one build makes.
	signinArtLookups = 24
	// signinRatingLookups caps the rating calls one trending build makes.
	signinRatingLookups = 24
	// signinArtTTL is how long a built answer is kept.
	signinArtTTL = 6 * time.Hour
	// signinArtEmptyTTL is how long an empty answer is kept (outside down).
	signinArtEmptyTTL = 5 * time.Minute
	// signinArtMinWidth is the smallest native backdrop width used at all.
	signinArtMinWidth = 1280
	// signinArtPerMinute is each address's calls a minute.
	signinArtPerMinute = 20
	// settingSigninLibraryArt is the this_app setting key.
	settingSigninLibraryArt = "signin_library_art"
)

// signinImage is one picture at its original size.
type signinImage struct {
	URL string `json:"url"`
	W   int    `json:"w"`
	H   int    `json:"h"`
}

// signinShot is one slide: a textless backdrop, an optional textless
// poster for tall screens, and the title's logo.
type signinShot struct {
	Backdrop signinImage  `json:"backdrop"`
	Poster   *signinImage `json:"poster,omitempty"`
	Logo     string       `json:"logo"`
}

type signinArtState struct {
	build sync.Mutex // one build at a time
	mu    sync.Mutex
	cache map[bool]signinArtCached // by "Library art on"
	hits  map[string][]time.Time

	// Test hooks; nil uses the metadata provider.
	images   func(ctx context.Context, kind, tmdbID string) (metadata.CardImages, error)
	trending func(ctx context.Context, kind string) ([]metadata.DiscoverItem, error)
	rating   func(ctx context.Context, kind, tmdbID string) (string, error)
	now      func() time.Time
}

type signinArtCached struct {
	shots []signinShot
	until time.Time
}

// kidSafeArt says a title with that rating may be shown before sign-in.
func kidSafeArt(kind, rating string) bool {
	switch kind {
	case "movie":
		return ratings.Allows("PG-13", kind, rating)
	case "show":
		return ratings.Allows("TV-PG", kind, rating)
	}
	return false
}

// signinLibraryArt reads "Show Library art on the sign-in page" (default on).
func (a *API) signinLibraryArt(ctx context.Context) bool {
	return a.uiSettings(ctx).SigninLibraryArt
}

func (st *signinArtState) clock() time.Time {
	if st.now != nil {
		return st.now()
	}
	return time.Now()
}

// allow counts one call from addr; false past the minute budget.
func (st *signinArtState) allow(addr string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.hits == nil {
		st.hits = map[string][]time.Time{}
	}
	now := st.clock()
	kept := st.hits[addr][:0]
	for _, t := range st.hits[addr] {
		if now.Sub(t) < time.Minute {
			kept = append(kept, t)
		}
	}
	if len(kept) >= signinArtPerMinute {
		st.hits[addr] = kept
		return false
	}
	st.hits[addr] = append(kept, now)
	if len(st.hits) > 4096 { // forget idle addresses
		for k, v := range st.hits {
			if len(v) == 0 || now.Sub(v[len(v)-1]) >= time.Minute {
				delete(st.hits, k)
			}
		}
	}
	return true
}

func remoteHost(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (a *API) signinArtRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/public/signin-art", a.signinArt)
}

// signinArt answers the sign-in page's pictures.
func (a *API) signinArt(w http.ResponseWriter, r *http.Request) {
	st := &a.signin
	if !st.allow(remoteHost(r)) {
		w.Header().Set("Retry-After", "60")
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many requests"})
		return
	}
	shots := a.signinShots(r.Context())
	w.Header().Set("Cache-Control", "private, max-age=600")
	writeJSON(w, http.StatusOK, map[string]any{"shots": shots})
}

// signinShots returns the cached answer, building it when stale.
func (a *API) signinShots(ctx context.Context) []signinShot {
	st := &a.signin
	lib := a.signinLibraryArt(ctx)
	read := func() ([]signinShot, bool) {
		st.mu.Lock()
		defer st.mu.Unlock()
		c, ok := st.cache[lib]
		return c.shots, ok && st.clock().Before(c.until)
	}
	if s, ok := read(); ok {
		return s
	}
	st.build.Lock()
	defer st.build.Unlock()
	if s, ok := read(); ok {
		return s
	}
	bctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
	defer cancel()
	var shots []signinShot
	if lib {
		shots = a.librarySigninShots(bctx)
	}
	if len(shots) < signinArtMin {
		shots = a.trendingSigninShots(bctx)
	}
	if shots == nil {
		shots = []signinShot{}
	}
	ttl := signinArtTTL
	if len(shots) == 0 {
		ttl = signinArtEmptyTTL
	}
	st.mu.Lock()
	if st.cache == nil {
		st.cache = map[bool]signinArtCached{}
	}
	st.cache[lib] = signinArtCached{shots: shots, until: st.clock().Add(ttl)}
	st.mu.Unlock()
	return shots
}

type signinRef struct{ kind, tmdbID string }

// librarySigninShots picks kid-safe Library movies and shows.
func (a *API) librarySigninShots(ctx context.Context) []signinShot {
	rows, err := a.s.Store.Read().QueryContext(ctx, `SELECT kind, rating, COALESCE(json_extract(source_ids, '$.tmdb'), '')
		FROM titles WHERE kind IN ('movie', 'show') AND state != 'removed' AND trim(COALESCE(rating, '')) != '' LIMIT 2000`)
	if err != nil {
		return nil
	}
	var refs []signinRef
	for rows.Next() {
		var kind, rating, id string
		if rows.Scan(&kind, &rating, &id) == nil && id != "" && kidSafeArt(kind, rating) {
			refs = append(refs, signinRef{kind, id})
		}
	}
	_ = rows.Close()
	rand.Shuffle(len(refs), func(i, j int) { refs[i], refs[j] = refs[j], refs[i] })
	return a.shotsFor(ctx, refs)
}

// trendingSigninShots picks kid-safe trending movies and shows.
func (a *API) trendingSigninShots(ctx context.Context) []signinShot {
	st := &a.signin
	list := st.trending
	if list == nil {
		d, ok := metadata.DiscovererFor(ctx, a.s.Metadata)
		if !ok {
			return nil
		}
		list = func(ctx context.Context, kind string) ([]metadata.DiscoverItem, error) {
			return d.Discover(ctx, "trending", kind, "", "")
		}
	}
	rate := st.rating
	if rate == nil {
		rt, ok := metadata.RaterFor(ctx, a.s.Metadata)
		if !ok {
			return nil
		}
		rate = rt.Rating
	}
	var items []metadata.DiscoverItem
	for _, kind := range []string{"movie", "show"} {
		got, err := list(ctx, kind)
		if err == nil {
			items = append(items, got...)
		}
	}
	// Most popular first, movies and shows together.
	slices.SortStableFunc(items, func(x, y metadata.DiscoverItem) int { return cmp.Compare(x.Popularity, y.Popularity) * -1 })
	var refs []signinRef
	for i, it := range items {
		if i >= signinRatingLookups || ctx.Err() != nil {
			break
		}
		if it.TMDBID == "" || (it.Kind != "movie" && it.Kind != "show") {
			continue
		}
		r, err := rate(ctx, it.Kind, it.TMDBID)
		if err == nil && kidSafeArt(it.Kind, r) {
			refs = append(refs, signinRef{it.Kind, it.TMDBID})
		}
	}
	return a.shotsFor(ctx, refs)
}

// shotsFor looks up each title's images until signinArtMax slides qualify.
func (a *API) shotsFor(ctx context.Context, refs []signinRef) []signinShot {
	look := a.signin.images
	if look == nil {
		src, ok := metadata.CardArterFor(ctx, a.s.Metadata)
		if !ok {
			return nil
		}
		look = func(ctx context.Context, kind, id string) (metadata.CardImages, error) {
			return src.CardImages(ctx, kind, id, "en")
		}
	}
	var out []signinShot
	for i, ref := range refs {
		if i >= signinArtLookups || len(out) >= signinArtMax || ctx.Err() != nil {
			break
		}
		im, err := look(ctx, ref.kind, ref.tmdbID)
		if err != nil {
			continue
		}
		if s, ok := pickSigninShot(im); ok {
			out = append(out, s)
		}
	}
	return out
}

// originalURL turns a sized TMDB picture address into its original one.
func originalURL(p string) (string, bool) {
	i := strings.LastIndex(p, "/")
	if i < 0 || !strings.HasPrefix(p, metadata.ImageBase+"/") {
		return "", false
	}
	file := p[i+1:]
	if file == "" || strings.ContainsAny(file, "?#\\") {
		return "", false
	}
	return metadata.ImageBase + "/original/" + file, true
}

// pickSigninShot picks the widest textless backdrop (at least
// signinArtMinWidth), the widest textless poster and the logo (English,
// else no language; PNG only). A title without a textless backdrop or a
// logo does not qualify.
func pickSigninShot(im metadata.CardImages) (signinShot, bool) {
	widest := func(list []metadata.CardImage, minW int) *signinImage {
		var best *metadata.CardImage
		for i := range list {
			x := &list[i]
			if x.Lang != nil || x.Width < minW || x.Height <= 0 {
				continue
			}
			if best == nil || x.Width > best.Width || (x.Width == best.Width && x.Vote > best.Vote) {
				best = x
			}
		}
		if best == nil {
			return nil
		}
		u, ok := originalURL(best.Path)
		if !ok {
			return nil
		}
		return &signinImage{URL: u, W: best.Width, H: best.Height}
	}
	back := widest(im.Backdrops, signinArtMinWidth)
	if back == nil {
		return signinShot{}, false
	}
	logo := ""
	for _, want := range []string{"en", ""} {
		for _, l := range im.Logos {
			lang := ""
			if l.Lang != nil {
				lang = strings.ToLower(*l.Lang)
			}
			if lang == want && strings.HasSuffix(strings.ToLower(l.Path), ".png") && strings.HasPrefix(l.Path, metadata.ImageBase+"/") {
				logo = l.Path
				break
			}
		}
		if logo != "" {
			break
		}
	}
	if logo == "" {
		return signinShot{}, false
	}
	return signinShot{Backdrop: *back, Poster: widest(im.Posters, 780), Logo: logo}, true
}
