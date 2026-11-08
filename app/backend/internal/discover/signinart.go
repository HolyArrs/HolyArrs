package discover

import (
	"cmp"
	"context"
	"math/rand/v2"
	"net"
	"net/http"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// The sign-in page's art: one wide picture at a time behind the card. The
// route is public (nobody is signed in yet), so the answer holds picture
// addresses and sizes only: no names, ids, counts or anything else.
//
// The pictures are TMDB's trending movies and shows this week, by
// popularity (like Overseerr). The owner can turn on "Show Library art on
// the sign-in page" (default off) to put Library titles first. There is no
// rating filter on this page's art.

const (
	// SigninArtMax is how many pictures one answer holds.
	SigninArtMax = 12
	// signinArtLookups caps the image list calls one build makes.
	signinArtLookups = 40
	// signinArtTTL is how long a built answer is kept.
	signinArtTTL = 6 * time.Hour
	// signinArtEmptyTTL is how long an empty answer is kept (outside down).
	signinArtEmptyTTL = 5 * time.Minute
	// signinArtRetryTTL is how long an empty answer caused by an error or
	// timeout is kept before the next try.
	signinArtRetryTTL = 15 * time.Second
	// signinArtWorkers is how many image lookups run at once.
	signinArtWorkers = 6
	// signinPickTTL is how long one title's pick is kept.
	signinPickTTL = 24 * time.Hour
	// signinArtMinWidth is the smallest native backdrop width used at all.
	signinArtMinWidth = 1920
	// SigninArtPerMinute is each address's calls a minute.
	SigninArtPerMinute = 20
	// signinPicPerMinute is each address's picture loads a minute.
	signinPicPerMinute = 240
	// signinPicPath is where the sign-in pictures are served, same-origin,
	// through the artwork cache (TMDB sends no CORS header, and the reveal
	// draws the picture on a canvas).
	signinPicPath = "/api/v1/public/signin-pic/t/p/"
)

// SigninImage is one picture at its original size.
type SigninImage struct {
	URL string `json:"url"`
	W   int    `json:"w"`
	H   int    `json:"h"`
}

// SigninShot is one slide: a textless backdrop, an optional textless
// poster for tall screens, and the title's logo.
type SigninShot struct {
	Backdrop SigninImage  `json:"backdrop"`
	Poster   *SigninImage `json:"poster,omitempty"`
	Logo     string       `json:"logo"`
}

type signinArtState struct {
	build sync.Mutex // one build at a time
	mu    sync.Mutex
	cache map[bool]signinArtCached // by "Library art on"
	picks map[signinRef]signinPick // per-title picks
	// failed marks a build that hit an error or timeout (not cached long).
	failed bool
	hits   map[string][]time.Time
	pics   map[string][]time.Time // picture loads, by address

	now func() time.Time // test hook; nil is time.Now
}

// Signin serves the sign-in page's art (/api/v1/public/signin-art and
// signin-pic) and the /login seed. Fields are set once before Routes.
type Signin struct {
	Store    *database.Store
	Metadata metadata.Provider
	// Artwork is the artwork cache, read per request (nil: no pictures).
	Artwork func() *library.ArtworkCache
	// ClientKey keys the rate limits (nil: the peer address).
	ClientKey func(*http.Request) string
	// LibraryArtOn reads "Show Library art on the sign-in page".
	LibraryArtOn func(context.Context) bool
	// ServeArtwork writes one cached picture file.
	ServeArtwork func(w http.ResponseWriter, r *http.Request, name, path string)

	// Test hooks; nil uses the metadata provider / a short download.
	Images   func(ctx context.Context, kind, tmdbID string) (metadata.CardImages, error)
	Trending func(ctx context.Context, kind string) ([]metadata.DiscoverItem, error)
	Fetch    func(ctx context.Context, url string) ([]byte, error)

	art  signinArtState
	seed signinSeedState
}

// SetNow replaces the clock (tests); call before any request.
func (s *Signin) SetNow(f func() time.Time) { s.art.now = f }

// HoldBuild blocks art builds until the returned func runs (tests).
func (s *Signin) HoldBuild() func() {
	s.art.build.Lock()
	return s.art.build.Unlock
}

type signinPick struct {
	shot  SigninShot
	ok    bool
	until time.Time
}

type signinArtCached struct {
	shots []SigninShot
	until time.Time
}

// signinLibraryArt reads "Show Library art on the sign-in page" (default off).
func (s *Signin) signinLibraryArt(ctx context.Context) bool {
	if s.LibraryArtOn == nil {
		return false
	}
	return s.LibraryArtOn(ctx)
}

func (st *signinArtState) clock() time.Time {
	if st.now != nil {
		return st.now()
	}
	return time.Now()
}

// allow counts one call from addr; false past the minute budget.
func (st *signinArtState) allow(addr string) bool {
	return st.allowIn(&st.hits, addr, SigninArtPerMinute)
}

// allowIn counts one call from addr in hits; false past limit a minute.
func (st *signinArtState) allowIn(hits *map[string][]time.Time, addr string, limit int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if *hits == nil {
		*hits = map[string][]time.Time{}
	}
	h := *hits
	now := st.clock()
	kept := h[addr][:0]
	for _, t := range h[addr] {
		if now.Sub(t) < time.Minute {
			kept = append(kept, t)
		}
	}
	if len(kept) >= limit {
		h[addr] = kept
		return false
	}
	h[addr] = append(kept, now)
	if len(h) > 4096 { // forget idle addresses
		for k, v := range h {
			if len(v) == 0 || now.Sub(v[len(v)-1]) >= time.Minute {
				delete(h, k)
			}
		}
	}
	return true
}

// signinClient keys the rate limit: the auth package's trusted-proxy
// client key, or the peer address when no Auth is in front (tests).
func (s *Signin) signinClient(r *http.Request) string {
	if s.ClientKey != nil {
		return s.ClientKey(r)
	}
	return remoteHost(r)
}

func remoteHost(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Signin) artwork() *library.ArtworkCache {
	if s.Artwork == nil {
		return nil
	}
	return s.Artwork()
}

// Routes registers the sign-in art routes and the /login seed.
func (s *Signin) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/public/signin-art", s.signinArt)
	// A literal pattern (= signinPicPath + "{size}/{file}"): the route tests read it from the source.
	m.HandleFunc("GET /api/v1/public/signin-pic/t/p/{size}/{file}", s.signinPic)
	webserver.SetLoginSeed(func(r *http.Request) []byte { return s.signinSeedJSON(r.Context()) })
	// Warm the cache in the background so the first visitor sees art.
	go func() {
		for _, wait := range []time.Duration{3 * time.Second, 30 * time.Second, 2 * time.Minute} {
			time.Sleep(wait)
			if s.Store == nil || len(s.signinShots(context.Background())) > 0 {
				return
			}
		}
	}()
}

// signinArt answers the sign-in page's pictures.
func (s *Signin) signinArt(w http.ResponseWriter, r *http.Request) {
	st := &s.art
	if !st.allow(s.signinClient(r)) {
		w.Header().Set("Retry-After", "60")
		webserver.WriteJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many requests"})
		return
	}
	shots := s.signinShots(r.Context())
	w.Header().Set("Cache-Control", "private, max-age=600")
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"shots": publicShots(shots)})
}

// signinPicSizes are the TMDB sizes the sign-in pictures are served at:
// thumbnails (w300, w342), posters (w500, w780), backdrops (w780, w1280)
// and logos (w500). The original (up to 3840 px) is served for picked
// backdrops and posters, for wide or high-density screens.
var signinPicSizes = map[string]bool{"w300": true, "w342": true, "w500": true, "w780": true, "w1280": true, "original": true}

// publicURL turns a TMDB picture address into its same-origin sign-in
// address; original becomes the largest served size for its kind.
func publicURL(u, largest string) string {
	rest, ok := strings.CutPrefix(u, metadata.ImageBase+"/")
	if !ok {
		return ""
	}
	size, file, ok := strings.Cut(rest, "/")
	if !ok || file == "" || strings.Contains(file, "/") {
		return ""
	}
	if !signinPicSizes[size] || size == "original" {
		size = largest
	}
	return signinPicPath + size + "/" + file
}

// publicShots is the answer as the browser sees it: same-origin addresses.
func publicShots(shots []SigninShot) []SigninShot {
	out := make([]SigninShot, 0, len(shots))
	for _, s := range shots {
		x := SigninShot{Backdrop: s.Backdrop, Logo: publicURL(s.Logo, "w500")}
		x.Backdrop.URL = publicURL(s.Backdrop.URL, "w1280")
		if s.Poster != nil {
			p := *s.Poster
			p.URL = publicURL(p.URL, "w780")
			x.Poster = &p
		}
		out = append(out, x)
	}
	return out
}

// signinPicFile reports whether file is one of the pictures a sign-in
// answer holds (the allow list: anything else is refused, so this public
// route is no open proxy).
// The original size is allowed for backdrops and posters, not logos.
func (st *signinArtState) signinPicFile(size, file string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	for _, c := range st.cache {
		for _, s := range c.shots {
			if path.Base(s.Backdrop.URL) == file {
				return true
			}
			if s.Poster != nil && path.Base(s.Poster.URL) == file {
				return true
			}
			if size == "original" {
				continue
			}
			if path.Base(s.Logo) == file {
				return true
			}
			if s.Poster != nil && path.Base(s.Poster.URL) == file {
				return true
			}
		}
	}
	return false
}

// signinPic serves one sign-in picture through the artwork cache.
func (s *Signin) signinPic(w http.ResponseWriter, r *http.Request) {
	st := &s.art
	if !st.allowIn(&st.pics, s.signinClient(r), signinPicPerMinute) {
		w.Header().Set("Retry-After", "60")
		http.Error(w, "too many requests", http.StatusTooManyRequests)
		return
	}
	size, file := r.PathValue("size"), r.PathValue("file")
	if !signinPicSizes[size] || !st.signinPicFile(size, file) || s.artwork() == nil {
		http.NotFound(w, r)
		return
	}
	src, ok := library.TMDBArtworkURL(metadata.ImageBase, size, file)
	if !ok {
		http.NotFound(w, r)
		return
	}
	c := s.artwork()
	name := library.ArtworkName(src)
	p, ok := c.Path(name)
	if !ok {
		if _, err := c.Fetch(r.Context(), src); err != nil {
			http.Error(w, "picture unavailable", http.StatusBadGateway)
			return
		}
		if p, ok = c.Path(name); !ok {
			http.NotFound(w, r)
			return
		}
	}
	s.ServeArtwork(w, r, name, p)
}

// signinShots returns the cached answer, building it when stale.
func (s *Signin) signinShots(ctx context.Context) []SigninShot {
	st := &s.art
	lib := s.signinLibraryArt(ctx)
	read := func() ([]SigninShot, bool) {
		st.mu.Lock()
		defer st.mu.Unlock()
		c, ok := st.cache[lib]
		return c.shots, ok && st.clock().Before(c.until)
	}
	if got, ok := read(); ok {
		return got
	}
	st.build.Lock()
	defer st.build.Unlock()
	if got, ok := read(); ok {
		return got
	}
	bctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
	defer cancel()
	st.mu.Lock()
	st.failed = false
	st.mu.Unlock()
	var shots []SigninShot
	if lib {
		shots = s.librarySigninShots(bctx)
	}
	if len(shots) < SigninArtMax {
		seen := map[string]bool{}
		for _, x := range shots {
			seen[x.Backdrop.URL] = true
		}
		for _, x := range s.trendingSigninShots(bctx) {
			if len(shots) < SigninArtMax && !seen[x.Backdrop.URL] {
				shots = append(shots, x)
			}
		}
	}
	if shots == nil {
		shots = []SigninShot{}
	}
	ttl := signinArtTTL
	st.mu.Lock()
	failed := st.failed || bctx.Err() != nil
	st.mu.Unlock()
	if len(shots) == 0 {
		ttl = signinArtEmptyTTL
		if failed {
			ttl = signinArtRetryTTL
		}
	}
	st.mu.Lock()
	if st.cache == nil {
		st.cache = map[bool]signinArtCached{}
	}
	st.cache[lib] = signinArtCached{shots: shots, until: st.clock().Add(ttl)}
	st.mu.Unlock()
	s.warmSigninSeed(shots)
	return shots
}

type signinRef struct{ kind, tmdbID string }

// librarySigninShots picks Library movies and shows.
func (s *Signin) librarySigninShots(ctx context.Context) []SigninShot {
	rows, err := s.Store.Read().QueryContext(ctx, `SELECT kind, COALESCE(json_extract(source_ids, '$.tmdb'), '')
		FROM titles WHERE kind IN ('movie', 'show') AND state != 'removed' LIMIT 2000`)
	if err != nil {
		s.art.fail()
		return nil
	}
	var refs []signinRef
	for rows.Next() {
		var kind, id string
		if rows.Scan(&kind, &id) == nil && id != "" {
			refs = append(refs, signinRef{kind, id})
		}
	}
	_ = rows.Close()
	rand.Shuffle(len(refs), func(i, j int) { refs[i], refs[j] = refs[j], refs[i] })
	return s.shotsFor(ctx, refs)
}

// trendingSigninShots picks trending movies and shows.
func (s *Signin) trendingSigninShots(ctx context.Context) []SigninShot {
	st := &s.art
	list := s.Trending
	if list == nil {
		d, ok := metadata.DiscovererFor(ctx, s.Metadata)
		if !ok {
			st.fail()
			return nil
		}
		list = func(ctx context.Context, kind string) ([]metadata.DiscoverItem, error) {
			return d.Discover(ctx, "trending", kind, "", "")
		}
	}
	var items []metadata.DiscoverItem
	for _, kind := range []string{"movie", "show"} {
		got, err := list(ctx, kind)
		if err != nil {
			st.fail()
			continue
		}
		items = append(items, got...)
	}
	// Most popular first, movies and shows together.
	slices.SortStableFunc(items, func(x, y metadata.DiscoverItem) int { return cmp.Compare(x.Popularity, y.Popularity) * -1 })
	var cand []signinRef
	for _, it := range items {
		if len(cand) >= signinArtLookups {
			break
		}
		if it.TMDBID != "" && (it.Kind == "movie" || it.Kind == "show") {
			cand = append(cand, signinRef{it.Kind, it.TMDBID})
		}
	}
	return s.shotsFor(ctx, cand)
}

// shotsFor looks up each title's images until SigninArtMax slides qualify.
func (s *Signin) shotsFor(ctx context.Context, refs []signinRef) []SigninShot {
	look := s.Images
	if look == nil {
		src, ok := metadata.CardArterFor(ctx, s.Metadata)
		if !ok {
			s.art.fail()
			return nil
		}
		look = func(ctx context.Context, kind, id string) (metadata.CardImages, error) {
			return src.CardImages(ctx, kind, id, "en")
		}
	}
	st := &s.art
	if len(refs) > signinArtLookups {
		refs = refs[:signinArtLookups]
	}
	picks := make([]signinPick, len(refs))
	parallel(ctx, len(refs), func(i int) {
		ref := refs[i]
		now := st.clock()
		st.mu.Lock()
		p, hit := st.picks[ref]
		st.mu.Unlock()
		if hit && now.Before(p.until) {
			picks[i] = p
			return
		}
		im, err := look(ctx, ref.kind, ref.tmdbID)
		if err != nil {
			st.fail()
			return
		}
		shot, ok := pickSigninShot(im)
		p = signinPick{shot: shot, ok: ok, until: now.Add(signinPickTTL)}
		picks[i] = p
		st.mu.Lock()
		if st.picks == nil || len(st.picks) > 4096 {
			st.picks = map[signinRef]signinPick{}
		}
		st.picks[ref] = p
		st.mu.Unlock()
	})
	var out []SigninShot
	for _, p := range picks {
		if p.ok && len(out) < SigninArtMax {
			out = append(out, p.shot)
		}
	}
	return out
}

// fail marks the running build as hit by an error.
func (st *signinArtState) fail() {
	st.mu.Lock()
	st.failed = true
	st.mu.Unlock()
}

// parallel runs fn for 0..n-1 on signinArtWorkers goroutines, skipping
// the rest once ctx is done.
func parallel(ctx context.Context, n int, fn func(i int)) {
	next := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < min(signinArtWorkers, n); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				fn(i)
			}
		}()
	}
	for i := 0; i < n && ctx.Err() == nil; i++ {
		next <- i
	}
	close(next)
	wg.Wait()
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

// pickSigninShot picks the textless backdrop (at least signinArtMinWidth
// wide) closest to 16:9, then the widest, then the best voted; the widest textless poster and the logo (English,
// else no language; PNG only). A title without a textless backdrop or a
// logo does not qualify.
func pickSigninShot(im metadata.CardImages) (SigninShot, bool) {
	// wide16x9 reports a 16:9 picture (within 2%).
	wide16x9 := func(x *metadata.CardImage) bool {
		d := float64(x.Width)/float64(x.Height) - 16.0/9.0
		return d < 0.036 && d > -0.036
	}
	widest := func(list []metadata.CardImage, minW int, prefer16x9 bool) *SigninImage {
		var best *metadata.CardImage
		for i := range list {
			x := &list[i]
			if x.Lang != nil || x.Width < minW || x.Height <= 0 {
				continue
			}
			if best != nil && prefer16x9 && wide16x9(x) != wide16x9(best) {
				if wide16x9(x) {
					best = x
				}
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
		return &SigninImage{URL: u, W: best.Width, H: best.Height}
	}
	back := widest(im.Backdrops, signinArtMinWidth, true)
	if back == nil {
		return SigninShot{}, false
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
		return SigninShot{}, false
	}
	return SigninShot{Backdrop: *back, Poster: widest(im.Posters, 780, false), Logo: logo}, true
}
