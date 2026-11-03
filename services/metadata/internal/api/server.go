package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/CoderLuii/holyarrs-metadata/internal/control"
	"github.com/CoderLuii/holyarrs-metadata/internal/extras"
	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
	"github.com/CoderLuii/holyarrs-metadata/internal/model"
	"github.com/CoderLuii/holyarrs-metadata/internal/providers"
	"github.com/CoderLuii/holyarrs-metadata/internal/refresh"
	"github.com/CoderLuii/holyarrs-metadata/internal/srctag"
	"github.com/CoderLuii/holyarrs-metadata/internal/stats"
	"github.com/CoderLuii/holyarrs-metadata/internal/upstream"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Storage interface {
	Get(context.Context, string) (model.Entry, bool, error)
	Put(context.Context, model.Entry) error
	Authorized(context.Context, string) (bool, error)
	Check(context.Context) error
	Ready(context.Context) error
}
type Fetcher interface {
	Fetch(context.Context, model.Query) (model.Result, error)
}
type Options struct {
	ProviderTTLs                                         map[string]time.Duration
	OperationTTLs                                        map[string]time.Duration
	EnabledProviders                                     map[string]bool
	NegativeTTL                                          time.Duration
	TTL, StaleTTL, LookupTimeout                         time.Duration
	PerMinute                                            float64
	Burst, PerTokenConcurrent, MaxConcurrent, TokenSlots int
	// ClientPerMinute and ClientBurst limit each client address (CF-Connecting-IP,
	// else the peer address) before any credential check.
	ClientPerMinute float64
	ClientBurst     int
	// ClientAuthPerMinute and ClientAuthBurst limit each client address for
	// reads with a working install credential (default twice the rate and
	// four times the burst): a household's installs behind one address open
	// pages in bursts. Every request first takes one from the ordinary
	// address limit; a working install credential gets it back, a failed
	// one keeps it spent.
	ClientAuthPerMinute float64
	ClientAuthBurst     int
	// TrustedProxies are peer networks allowed to set CF-Connecting-IP.
	TrustedProxies []*net.IPNet
	Stats          *stats.Recorder
	StatsToken     string
	// Registration opens POST /v1/installations: each call issues a new
	// random self-registered credential, after the client-address and
	// admission gates, limited in total (RegisterGlobalPerHour, default 600)
	// and per address group (RegisterPerHour, default 3; IPv6 by /64). The
	// store must implement Registrar. Self-registered credentials reach
	// only the lookup routes, expire after RegisteredIdle without use
	// (default 90 days), are capped at RegisteredMax active (default
	// 100000, least recently used evicted) and share one limit per source
	// address group.
	//
	// Wider groups have their own hourly limits too: RegisterNetworkPerHour
	// (default 10) per IPv4 /24 and IPv6 /48, and RegisterWidePerHour
	// (default 30) per IPv6 /32. RegisterReserved (default a sixth) of the
	// global budget is kept for networks (IPv4 /24, IPv6 /32) that have not
	// registered within the hour, so a flood cannot lock new installs out.
	// Credentials used within RegisteredProtect (default 30 days) are never
	// evicted; a full table answers 429 registry_full instead.
	Registration                           bool
	RegisterPerHour, RegisterGlobalPerHour int
	RegisterNetworkPerHour                 int
	RegisterWidePerHour                    int
	RegisterReserved                       int
	RegisteredProtect                      time.Duration
	RegisteredIdle                         time.Duration
	RegisteredMax                          int
	// ServiceTop serves GET /v1/lists/service-top to any lookup credential.
	ServiceTop ServiceTop
	// Music serves GET /v1/lists/music-trending to any lookup credential.
	Music MusicLists
	// Slim answers MusicBrainz detail lookups (artist, release-group,
	// release, recording) from the slim dump copy, with its dump date;
	// false sends the lookup on to the stored answers and the live queue.
	// nil: no slim copy.
	Slim func(ctx context.Context, kind, mbid string) ([]byte, string, bool)
	// SlimMissing reports an id the loaded slim copy does not hold (false
	// with no copy loaded, or for a kind it does not carry). An anonymous
	// ask for such an id is answered 404 at once, never queued. nil: no
	// check.
	SlimMissing func(ctx context.Context, kind, mbid string) bool
	// Similar serves GET /v1/music/similar/{mbid} to any lookup credential.
	Similar SimilarArtists
	// Popularity adds an artist's kworb figures to its MusicBrainz answer
	// (data.popularity) and serves GET /v1/music/charts/rising.
	Popularity ArtistPopularity
	// MusicData serves GET /v1/music/top-songs/{mbid},
	// /v1/music/discography/{mbid} and /v1/music/album-details to any
	// lookup credential.
	MusicData MusicData
	// Discog holds every artist's full release-group list (put into the
	// artist answer and /v1/music/discography/{mbid}) and serves GET
	// /v1/music/album/{release-group mbid}; its walks run on Queue.
	Discog Discographies
	// Covers serves GET /v1/music/cover/{mbid} to any lookup credential.
	Covers AlbumCovers
	// ChartsReady is the chart gate (musictrend.Prefetch.Ready): while it
	// is false, charts, genres, new releases, trending lists and shelves answer
	// "building". nil: always open.
	ChartsReady func() bool
	// NewMusic serves GET /v1/music/new to any lookup credential.
	NewMusic NewMusicWindows
	// Shelves serves GET /v1/music/lists/shelves (world lists as album and
	// artist shelves) to any lookup credential.
	Shelves MusicShelves
	// Pools serves GET /v1/music/pools: the music catalog bundle for the
	// row templates, the same for every install.
	Pools MusicPools
	// Trailers serves GET /v1/trailers/{movie|tv}/{tmdb_id} to any lookup
	// credential (self-registered ones included) and adds trailer health
	// to /v1/stats.
	Trailers Trailers
	// Anime serves GET /v1/anime/map to any lookup credential.
	Anime AnimeMap
	// Fanart serves GET /v1/art/{movie|tv|music}/{id}; Ratings serves
	// GET /v1/ratings/{movie|tv}/{tmdb_id}. Each shows its state in
	// /v1/stats (disabled_no_key when its key is unset).
	Fanart  ArtSource
	Ratings RatingsSource
	// GenreTiles renders /v1/music/genres tile art; nil keeps flat hues.
	GenreTiles *GenreTiles
	// Wikidata serves /v1/wikidata/... to any lookup credential.
	Wikidata Wikidata
	// Control serves GET /v1/control (the kill switch), public and
	// cacheable for 60 s.
	Control *control.Switch
	// Queue, when set, takes MusicBrainz detail misses: a P0 job the
	// request waits on for up to QueueWait (default 800 ms), then a
	// partial answer. OnFetched runs after each queued fetch.
	Queue     FetchQueue
	QueueWait time.Duration
	OnFetched func(kind, mbid string)
	// Images serves /v1/img/... (the picture store).
	Images *Images
	// TMDBImages is TMDB's picture network (default TMDBImageBase) and
	// TMDBImageClient the client asking it (tests point both at fakes).
	TMDBImages      string
	TMDBImageClient *http.Client
	// PersonPicture answers our own address for a person's Commons photo
	// (its thumbnail address in image), "" to keep image as it is.
	PersonPicture func(ctx context.Context, tmdb, image string) string
	// Pictures answers the stored picture records an artist answer
	// (photo, wide, logo) or an album answer (cover) carries as
	// data.pictures; nil adds none.
	Pictures func(ctx context.Context, kind, mbid string) map[string]imgstore.Info
	// PicturePending reports an artist's photo or an album's cover not
	// stored yet and not known missing: the answer is then marked partial
	// (a minute at the edge) so the picture shows once stored. nil: never.
	PicturePending func(ctx context.Context, kind, mbid string) bool
	// LiveTag keeps the full source tag of a live MusicBrainz answer
	// (with the egress address) internally; it is never served.
	LiveTag func(kind, mbid string, t srctag.Tag)
	// Packages serves /v1/music/packages (sync) and /v1/music/packages/{mbid};
	// WantPackage records an ask (id only) from /v1/music/package-want/{mbid}.
	Packages    Packages
	WantPackage func(ctx context.Context, mbid string) error
	// Videos serves /v1/music/videos/{mbid}; WantVideos queues a collection.
	Videos     HeroVideos
	WantVideos func(mbid string)
	// StatusExtra adds the "speed" object to /v1/stats.
	StatusExtra func(ctx context.Context, window time.Duration) map[string]any
	// Refresh turns on change-driven refresh: answers are always served
	// from the store, and stale ones (marked by a feed, past verify-after,
	// or a title page unverified for NudgeAfter) refresh in the background.
	// Feed-covered answers verify after Backstop (TMDB, and MusicBrainz
	// when MusicFeed is on); MusicBrainz without the feed after MusicTTL.
	Refresh         *refresh.Tracker
	RefreshSnapshot func(context.Context) refresh.Stats
	MusicFeed       bool
	Backstop        time.Duration
	MusicTTL        time.Duration
	NudgeAfter      time.Duration
}

// Registrar keeps self-registered credentials apart from operator-issued
// ones, by hash only.
type Registrar interface {
	RegisterToken(ctx context.Context, token, source, network string, maxActive int, idle, protect time.Duration) error
	Registered(ctx context.Context, token string, idle time.Duration) (source string, ok bool, err error)
}
type allowance struct {
	tokens  float64
	updated time.Time
	active  int
}
type flight struct {
	done      chan struct{}
	entry     model.Entry
	err       error
	persisted bool
}
type Server struct {
	store                                                            Storage
	fetcher                                                          Fetcher
	opts                                                             Options
	admission                                                        chan struct{}
	mu                                                               sync.Mutex
	tokens                                                           map[string]*allowance
	clients                                                          map[string]*allowance
	flights                                                          map[string]*flight
	registrations                                                    map[string]*allowance
	sources                                                          map[string]*allowance
	registered, reserved                                             allowance
	background                                                       chan struct{}
	absent                                                           absentCache
	requests, hits, misses, failures, writeFailures, stale, rejected atomic.Int64
}

func New(s Storage, f Fetcher, o Options) *Server {
	if o.TTL <= 0 {
		o.TTL = 24 * time.Hour
	}
	if o.NegativeTTL <= 0 {
		o.NegativeTTL = 5 * time.Minute
	}
	if o.StaleTTL < 0 {
		o.StaleTTL = 0
	}
	if o.LookupTimeout <= 0 {
		o.LookupTimeout = 15 * time.Second
	}
	if o.PerMinute <= 0 {
		o.PerMinute = 60
	}
	if o.Burst <= 0 {
		o.Burst = 10
	}
	if o.PerTokenConcurrent <= 0 {
		o.PerTokenConcurrent = 4
	}
	if o.MaxConcurrent <= 0 {
		o.MaxConcurrent = 100
	}
	if o.TokenSlots <= 0 {
		o.TokenSlots = 10000
	}
	if o.ClientPerMinute <= 0 {
		o.ClientPerMinute = max(120, 2*o.PerMinute)
	}
	if o.ClientBurst <= 0 {
		o.ClientBurst = max(30, 3*o.Burst)
	}
	if o.ClientAuthPerMinute <= 0 {
		o.ClientAuthPerMinute = 2 * o.ClientPerMinute
	}
	if o.ClientAuthBurst <= 0 {
		o.ClientAuthBurst = 4 * o.ClientBurst
	}
	if o.RegisterPerHour <= 0 {
		o.RegisterPerHour = 3
	}
	if o.RegisterGlobalPerHour <= 0 {
		o.RegisterGlobalPerHour = 600
	}
	if o.RegisteredIdle <= 0 {
		o.RegisteredIdle = 90 * 24 * time.Hour
	}
	if o.RegisteredMax <= 0 {
		o.RegisteredMax = 100000
	}
	if o.RegisterNetworkPerHour <= 0 {
		o.RegisterNetworkPerHour = 10
	}
	if o.RegisterWidePerHour <= 0 {
		o.RegisterWidePerHour = 30
	}
	if o.RegisterReserved <= 0 || o.RegisterReserved >= o.RegisterGlobalPerHour {
		o.RegisterReserved = o.RegisterGlobalPerHour / 6
	}
	if o.RegisteredProtect <= 0 {
		o.RegisteredProtect = 30 * 24 * time.Hour
	}
	if o.Backstop <= 0 {
		o.Backstop = refresh.DefaultBackstop
	}
	if o.MusicTTL <= 0 {
		o.MusicTTL = 30 * 24 * time.Hour
	}
	if o.NudgeAfter <= 0 {
		o.NudgeAfter = 6 * time.Hour
	}
	return &Server{background: make(chan struct{}, 8), store: s, fetcher: f, opts: o, admission: make(chan struct{}, o.MaxConcurrent), tokens: map[string]*allowance{}, clients: map[string]*allowance{}, flights: map[string]*flight{}, registrations: map[string]*allowance{}, sources: map[string]*allowance{}, registered: allowance{tokens: float64(o.RegisterGlobalPerHour - o.RegisterReserved), updated: time.Now()}, reserved: allowance{tokens: float64(o.RegisterReserved), updated: time.Now()}}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func problem(w http.ResponseWriter, status int, code string, retry time.Duration) {
	seconds := 0
	if retry > 0 {
		seconds = int((retry + time.Second - 1) / time.Second)
		w.Header().Set("Retry-After", strconv.Itoa(seconds))
	}
	writeJSON(w, status, map[string]any{"error": map[string]any{"code": code, "retry_after_seconds": seconds}})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

// clientAddress uses CF-Connecting-IP only when the peer is a trusted proxy.
func (s *Server) clientAddress(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer := net.ParseIP(host)
	for _, network := range s.opts.TrustedProxies {
		if peer != nil && network.Contains(peer) {
			if ip := net.ParseIP(strings.TrimSpace(r.Header.Get("CF-Connecting-IP"))); ip != nil {
				return ip.String()
			}
			break
		}
	}
	return host
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" || r.URL.Path == "/v1/stats" || r.URL.Path == "/v1/stats/items" || s.opts.Stats == nil {
		s.serve(w, r)
		return
	}
	started := time.Now()
	sw := &statusWriter{ResponseWriter: w}
	s.serve(sw, r)
	if sw.status == 0 {
		sw.status = 200
	}
	s.opts.Stats.Request(sw.status, sw.Header().Get("X-Metadata-Cache"), time.Since(started))
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	s.requests.Add(1)
	register := r.URL.Path == "/v1/installations" && s.opts.Registration
	if r.Method != "GET" && !(register && r.Method == "POST") {
		w.Header().Set("Allow", "GET")
		problem(w, 405, "method_not_allowed", 0)
		return
	}
	if r.URL.Path == "/healthz" {
		writeJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	if r.URL.Path == "/readyz" {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if s.store.Ready(ctx) != nil {
			problem(w, 503, "storage_unavailable", time.Second)
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	address := s.clientAddress(r)
	// Every request takes one from its address's ordinary limit first, a
	// wrong credential on any route included. A read with an install
	// credential (credentialRoute) gets it back once the credential checks
	// out and is then limited by the larger ClientAuth limit; a failed
	// credential keeps it spent, so guesses run out at the ordinary burst.
	bearer := strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") && credentialRoute(r.URL.Path) && !register
	if !s.allowClient(address) {
		s.rejected.Add(1)
		problem(w, 429, "client_limit", time.Minute/time.Duration(max(1, int(s.opts.ClientPerMinute))))
		return
	}
	if r.URL.Path == "/v1/stats" {
		s.serveStats(w, r)
		return
	}
	if r.URL.Path == "/v1/stats/items" {
		s.serveStatsItems(w, r)
		return
	}
	select {
	case s.admission <- struct{}{}:
		defer func() { <-s.admission }()
	default:
		s.rejected.Add(1)
		problem(w, 503, "service_busy", time.Second)
		return
	}
	if len(r.RequestURI) > 4096 || r.ContentLength > 0 {
		problem(w, 400, "invalid_request", 0)
		return
	}
	if register {
		if r.Method != "POST" {
			w.Header().Set("Allow", "POST")
			problem(w, 405, "method_not_allowed", 0)
			return
		}
		s.serveRegister(w, r)
		return
	}
	if r.URL.Path == "/metrics" {
		if !s.statsAuthorized(r) {
			problem(w, 401, "unauthorized", 0)
			return
		}
		writeJSON(w, 200, map[string]int64{"requests": s.requests.Load(), "cache_hits": s.hits.Load(), "cache_misses": s.misses.Load(), "upstream_failures": s.failures.Load(), "cache_write_failures": s.writeFailures.Load(), "stale_served": s.stale.Load(), "rejected": s.rejected.Load(), "inflight": int64(len(s.admission))})
		return
	}
	if policy := edgePolicy(r.URL.Path); policy != "" {
		w = &edgeWriter{ResponseWriter: w, policy: policy}
		if r.Header.Get("Authorization") == "" {
			// Shared catalogue route without a credential: public by
			// design (rule 7); only the client-address limit applies.
			r = r.WithContext(context.WithValue(r.Context(), callerKey{}, caller{registered: true, who: "anon"}))
			s.route(w, r)
			return
		}
	} else if policy := edgeAuthPolicy(r.URL.Path); policy != "" {
		w = &edgeWriter{ResponseWriter: w, policy: policy}
	}
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") || len(auth) > 512 {
		problem(w, 401, "unauthorized", 0)
		return
	}
	token := strings.TrimPrefix(auth, "Bearer ")
	ok, err := s.store.Authorized(r.Context(), token)
	if err != nil {
		problem(w, 503, "authorization_unavailable", time.Second)
		return
	}
	source := ""
	if !ok && s.opts.Registration {
		// Self-registered credentials: their own limit, plus a larger
		// total per source address group.
		if reg, isReg := s.store.(Registrar); isReg {
			src, found, err := reg.Registered(r.Context(), token, s.opts.RegisteredIdle)
			if err != nil {
				problem(w, 503, "authorization_unavailable", time.Second)
				return
			}
			ok, source = found, src
		}
	}
	if !ok {
		problem(w, 401, "unauthorized", 0)
		return
	}
	if bearer {
		s.refundClient(address)
	}
	if bearer && !s.allowBucket("auth|"+addressGroup(address), s.opts.ClientAuthPerMinute, s.opts.ClientAuthBurst) {
		s.rejected.Add(1)
		problem(w, 429, "client_limit", time.Minute/time.Duration(max(1, int(s.opts.ClientAuthPerMinute))))
		return
	}
	release, allowed := s.allowFor(token, r.Header.Get("X-Fetch") == "ahead")
	if allowed && source != "" && !s.allowSource(source) {
		release()
		allowed = false
	}
	if !allowed {
		s.rejected.Add(1)
		problem(w, 429, "installation_limit", time.Minute/time.Duration(max(1, int(s.opts.PerMinute))))
		return
	}
	defer release()
	who := "token:" + tokenKey(token)
	if source != "" {
		if !registeredRoute(r.URL.Path) {
			problem(w, 403, "scope", 0)
			return
		}
		who = "net:" + networkOf(source)
	}
	r = r.WithContext(context.WithValue(r.Context(), callerKey{}, caller{registered: source != "", who: who}))
	s.route(w, r)
}

// route answers a request that passed the credential checks (or a shared
// catalogue request without a credential).
func (s *Server) route(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/v1/control" {
		s.serveControl(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/v1/img/") {
		s.serveImage(w, r)
		return
	}
	if r.URL.Path == "/v1/music/pictures" {
		s.serveMusicPictures(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/v1/music/videos/") {
		s.serveVideos(w, r)
		return
	}
	if r.URL.Path == "/v1/lists/service-top" {
		s.serveServiceTop(w, r)
		return
	}
	if r.URL.Path == "/v1/lists/music-trending" {
		s.serveMusicTrending(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/v1/music/similar/") {
		s.serveMusicSimilar(w, r)
		return
	}
	if r.URL.Path == "/v1/music/charts/rising" {
		s.serveMusicRising(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/v1/music/top-songs/") {
		s.serveMusicTopSongs(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/v1/music/discography/") {
		s.serveMusicDiscography(w, r)
		return
	}
	if r.URL.Path == "/v1/music/packages" || strings.HasPrefix(r.URL.Path, "/v1/music/packages/") {
		if s.opts.Packages == nil {
			problem(w, 404, "list_unavailable", 0)
			return
		}
		if r.URL.Path == "/v1/music/packages" {
			s.servePackages(w, r)
		} else {
			s.servePackage(w, r, strings.ToLower(strings.TrimPrefix(r.URL.Path, "/v1/music/packages/")))
		}
		return
	}
	if strings.HasPrefix(r.URL.Path, "/v1/music/package-want/") {
		s.servePackageWant(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/v1/music/album/") {
		s.serveMusicAlbum(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/v1/music/deezer/") {
		s.serveMusicDeezer(w, r)
		return
	}
	switch r.URL.Path {
	case "/v1/music/charts":
		s.serveMusicCharts(w, r)
		return
	case "/v1/music/genres":
		s.serveMusicGenres(w, r)
		return
	case "/v1/music/suggest":
		s.serveMusicSuggest(w, r)
		return
	case "/v1/music/releases":
		s.serveMusicReleases(w, r)
		return
	case "/v1/music/album-details":
		s.serveMusicAlbumDetails(w, r)
		return
	case "/v1/music/similar-set":
		s.serveMusicSimilarSet(w, r)
		return
	case "/v1/music/artist-data":
		s.serveMusicArtistData(w, r)
		return
	case "/v1/music/popularity":
		s.serveMusicPopularity(w, r)
		return
	}
	if r.URL.Path == "/v1/music/new" {
		s.serveMusicNew(w, r)
		return
	}
	if r.URL.Path == "/v1/music/pools" {
		s.serveMusicPools(w, r)
		return
	}
	if r.URL.Path == "/v1/music/lists/shelves" {
		s.serveMusicShelves(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/v1/music/genres/tile/") {
		s.serveMusicGenreTile(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/v1/music/cover/") {
		s.serveMusicCover(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/v1/trailers/") {
		s.serveTrailer(w, r)
		return
	}
	if r.URL.Path == "/v1/anime/map" {
		s.serveAnimeMap(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/v1/wikidata/") {
		s.serveWikidata(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/v1/art/") {
		s.serveArt(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/v1/ratings/") {
		s.serveRatings(w, r)
		return
	}
	q, err := parseQuery(r.URL)
	if err != nil {
		problem(w, 400, "invalid_query", 0)
		return
	}
	if s.opts.EnabledProviders != nil && !s.opts.EnabledProviders[q.Provider] {
		problem(w, 503, "provider_unavailable", time.Second)
		return
	}
	if s.opts.Popularity != nil && q.Provider == "musicbrainz" && q.Kind == "artist" && q.Operation != "search" && r.Header.Get("X-Fetch") != "ahead" {
		// An artist page opened (not a fetch ahead): noted only, so the
		// artist goes to the front of the next crawl; nothing is read.
		s.opts.Popularity.Opened(q.ID)
	}
	if e, ok := s.slim(r.Context(), q); ok {
		s.hits.Add(1)
		s.respond(w, q, e, false, "slim")
		return
	}
	key := q.Key()
	cached, found, err := s.store.Get(r.Context(), key)
	if err != nil {
		problem(w, 503, "storage_unavailable", time.Second)
		return
	}
	if found && providers.TVNeedsTrailer(q, cached.Result.Data) {
		// Stored before the season-trailer walk, or no trailer then: short.
		if short := cached.FetchedAt.Add(15 * time.Minute); short.Before(cached.ExpiresAt) {
			cached.ExpiresAt = short
		}
	}
	if found && s.opts.Refresh != nil {
		s.serveStored(w, q, cached)
		return
	}
	if found && time.Now().Before(cached.ExpiresAt) {
		s.hits.Add(1)
		s.respond(w, q, cached, false, "hit")
		return
	}
	if s.opts.Queue != nil && q.Provider == "musicbrainz" && q.ID != "" && !found {
		// An anonymous ask for an id the slim copy lacks never reaches
		// the live queue (random ids would fill its P0 lane).
		if Anonymous(r.Context()) && s.SlimAbsent(r.Context(), q.Kind, q.ID) {
			problem(w, 404, "not_found", 0)
			return
		}
		s.serveQueued(w, r, q)
		return
	}
	s.misses.Add(1)
	e, persisted, err := s.load(r.Context(), q, false)
	if err != nil {
		s.failures.Add(1)
		var fe *model.FetchError
		staleAllowed := !errors.As(err, &fe) || (fe.Code != "provider_credentials" && fe.Code != "invalid_query" && fe.Code != "provider_unavailable")
		// A spent upstream budget serves any cached answer (stale while
		// revalidate); other failures only within StaleTTL.
		spent := fe != nil && fe.Code == "upstream_budget"
		if found && staleAllowed && (spent || s.opts.StaleTTL > 0 && time.Now().Before(cached.ExpiresAt.Add(s.opts.StaleTTL))) {
			s.stale.Add(1)
			s.respond(w, q, cached, true, "stale")
			return
		}
		if s.opts.Stats != nil {
			code := "lookup_failed"
			if errors.As(err, &fe) {
				code = fe.Code
			}
			s.opts.Stats.ProviderError(q.Provider, code)
		}
		if errors.As(err, &fe) {
			status := 503
			if fe.Code == "invalid_query" {
				status = 400
			}
			if fe.Code == "upstream_status" && fe.Status == 404 {
				status = 404
			}
			retry := fe.RetryAfter
			if status == 503 && retry <= 0 {
				retry = time.Second
			}
			problem(w, status, fe.Code, retry)
			return
		}
		problem(w, 503, "lookup_failed", time.Second)
		return
	}
	state := "miss"
	if !persisted {
		state = "bypass"
	}
	s.respond(w, q, e, false, state)
}

func (s *Server) allow(token string) (func(), bool) { return s.allowFor(token, false) }

// allowFor takes one request from an installation's allowance. Fetch-ahead
// (X-Fetch: ahead) never spends the last third of the burst or the last
// two concurrent slots: a page someone opens right after a busy row still
// gets its answers.
func (s *Server) allowFor(token string, ahead bool) (func(), bool) {
	h := sha256.Sum256([]byte(token))
	key := hex.EncodeToString(h[:])
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.tokens[key]
	if a == nil {
		if len(s.tokens) >= s.opts.TokenSlots {
			oldest, oldestKey := now, ""
			for k, v := range s.tokens {
				if v.active == 0 && now.Sub(v.updated) > time.Hour {
					delete(s.tokens, k)
				} else if v.active == 0 && !v.updated.After(oldest) {
					oldest, oldestKey = v.updated, k
				}
			}
			if len(s.tokens) >= s.opts.TokenSlots && oldestKey != "" {
				delete(s.tokens, oldestKey)
			}
			// Every slot busy with a request in flight: refuse briefly.
			if len(s.tokens) >= s.opts.TokenSlots {
				return nil, false
			}
		}
		a = &allowance{tokens: float64(s.opts.Burst), updated: now}
		s.tokens[key] = a
	}
	a.tokens = min(float64(s.opts.Burst), a.tokens+now.Sub(a.updated).Minutes()*s.opts.PerMinute)
	a.updated = now
	need, slots := 1.0, s.opts.PerTokenConcurrent
	if ahead {
		need += float64(s.opts.Burst) / 3
		slots = max(1, slots-2)
	}
	if a.tokens < need || a.active >= slots {
		return nil, false
	}
	a.tokens--
	a.active++
	return func() { s.mu.Lock(); a.active--; s.mu.Unlock() }, true
}

func (s *Server) allowClient(address string) bool {
	return s.allowBucket(addressGroup(address), s.opts.ClientPerMinute, s.opts.ClientBurst)
}

// refundClient gives back the one an install-credential read took from
// its address's ordinary limit.
func (s *Server) refundClient(address string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a := s.clients[addressGroup(address)]; a != nil {
		a.tokens = min(float64(s.opts.ClientBurst), a.tokens+1)
	}
}

// credentialRoute reports a route whose reads check an install credential
// in the store (every /v1/ route but the statistics and registration);
// only these get the credentialed address limit. /metrics and the
// statistics check the statistics token instead and stay on the ordinary
// limit.
func credentialRoute(path string) bool {
	return strings.HasPrefix(path, "/v1/") && path != "/v1/stats" && path != "/v1/stats/items" && path != "/v1/installations"
}

// allowBucket takes one from key's bucket (rate a minute, burst).
func (s *Server) allowBucket(address string, rate float64, burst int) bool {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.clients[address]
	if a == nil {
		if len(s.clients) >= s.opts.TokenSlots {
			oldest, oldestKey := now, ""
			for k, v := range s.clients {
				if now.Sub(v.updated) > 10*time.Minute {
					delete(s.clients, k)
				} else if !v.updated.After(oldest) {
					oldest, oldestKey = v.updated, k
				}
			}
			if len(s.clients) >= s.opts.TokenSlots && oldestKey != "" {
				delete(s.clients, oldestKey)
			}
		}
		a = &allowance{tokens: float64(burst), updated: now}
		s.clients[address] = a
	}
	a.tokens = min(float64(burst), a.tokens+now.Sub(a.updated).Minutes()*rate)
	a.updated = now
	if a.tokens < 1 {
		return false
	}
	a.tokens--
	return true
}

var rawItemPath = regexp.MustCompile(`^/(movie|tv)/[1-9][0-9]*$`)

// itemKind is the item kind a stored answer counts as: a title detail, or a
// raw TMDB read of a bare movie or TV record. Searches, seasons, lists and
// other raw reads are not items.
func itemKind(q model.Query) string {
	if q.Operation == model.OperationTMDB {
		if m := rawItemPath.FindStringSubmatch(q.Path); m != nil {
			if m[1] == "tv" {
				return "show"
			}
			return "movie"
		}
		return ""
	}
	if q.Operation == "detail" {
		return q.Kind
	}
	return ""
}

func itemTitle(data json.RawMessage) string {
	var v struct {
		Title string `json:"title"`
		Name  string `json:"name"`
	}
	if json.Unmarshal(data, &v) != nil {
		return ""
	}
	if v.Title != "" {
		return v.Title
	}
	return v.Name
}

// serveStatsItems lists items recently added to the cache (stats token only).
// Query: window_hours 1-168 (default 24), kind movies|shows|music, limit
// 1-100 (default 50), offset 0-10000.
func (s *Server) serveStatsItems(w http.ResponseWriter, r *http.Request) {
	if s.opts.Stats == nil || !s.statsAuthorized(r) {
		problem(w, 401, "unauthorized", 0)
		return
	}
	v := r.URL.Query()
	window, kind, limit, offset := 24*time.Hour, "", 50, 0
	for k, vals := range v {
		if len(vals) != 1 {
			problem(w, 400, "invalid_query", 0)
			return
		}
		raw := vals[0]
		n, err := strconv.Atoi(raw)
		switch k {
		case "window_hours":
			if err != nil || n < 1 || n > 168 {
				problem(w, 400, "invalid_query", 0)
				return
			}
			window = time.Duration(n) * time.Hour
		case "kind":
			if raw != "" && raw != "movies" && raw != "shows" && raw != "music" {
				problem(w, 400, "invalid_query", 0)
				return
			}
			kind = raw
		case "limit":
			if err != nil || n < 1 || n > 100 {
				problem(w, 400, "invalid_query", 0)
				return
			}
			limit = n
		case "offset":
			if err != nil || n < 0 || n > 10000 {
				problem(w, 400, "invalid_query", 0)
				return
			}
			offset = n
		default:
			problem(w, 400, "invalid_query", 0)
			return
		}
	}
	page, err := s.opts.Stats.Items(window, kind, limit, offset)
	if err != nil {
		problem(w, 503, "statistics_unavailable", time.Second)
		return
	}
	writeJSON(w, 200, page)
}

func (s *Server) serveStats(w http.ResponseWriter, r *http.Request) {
	if s.opts.Stats == nil || !s.statsAuthorized(r) {
		problem(w, 401, "unauthorized", 0)
		return
	}
	window := 24 * time.Hour
	v := r.URL.Query()
	for k := range v {
		if k != "window_hours" {
			problem(w, 400, "invalid_query", 0)
			return
		}
	}
	if raw := v.Get("window_hours"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 168 {
			problem(w, 400, "invalid_query", 0)
			return
		}
		window = time.Duration(n) * time.Hour
	}
	snap, err := s.opts.Stats.Snapshot(window)
	if err != nil {
		problem(w, 503, "statistics_unavailable", time.Second)
		return
	}
	if s.opts.Trailers != nil {
		t := s.opts.Trailers.Stats()
		snap.Trailers = &t
	}
	if s.opts.Wikidata != nil {
		snap.Wikidata = s.opts.Wikidata.Stats()
	}
	if s.opts.ServiceTop != nil {
		snap.ServiceTop = s.opts.ServiceTop.Stats()
	}
	ex := map[string]extras.Status{"fanart": {State: extras.StateDisabledNoKey}, "mdblist": {State: extras.StateDisabledNoKey}}
	if s.opts.Fanart != nil {
		ex["fanart"] = s.opts.Fanart.Status()
	}
	if s.opts.Ratings != nil {
		ex["mdblist"] = s.opts.Ratings.Status()
	}
	snap.Extras = ex
	s.mu.Lock()
	snapshot := s.opts.RefreshSnapshot
	s.mu.Unlock()
	if snapshot != nil {
		snap.Refresh = snapshot(r.Context())
	}
	if s.opts.StatusExtra != nil {
		// VM resources, queues, picture store, shared lists, hero videos.
		writeJSON(w, 200, struct {
			stats.Snapshot
			Speed map[string]any `json:"speed"`
		}{snap, s.opts.StatusExtra(r.Context(), window)})
		return
	}
	writeJSON(w, 200, snap)
}

// serveStored answers from the store (stale while revalidate) and queues
// one background refresh when the answer is stale, past verify-after, or a
// title page not verified within NudgeAfter.
func (s *Server) serveStored(w http.ResponseWriter, q model.Query, cached model.Entry) {
	tr, key, now := s.opts.Refresh, cached.Key, time.Now()
	ctx := context.Background()
	tr.Touch(ctx, key)
	verified, stale, tracked := tr.State(ctx, key)
	if !tracked {
		verified = cached.FetchedAt
	}
	expired := !now.Before(cached.ExpiresAt)
	nudge := refresh.TitlePage(q) && now.Sub(verified) > s.opts.NudgeAfter
	state := "hit"
	if expired || stale || nudge {
		s.revalidate(q)
		if expired || stale {
			state = "stale"
			s.stale.Add(1)
		}
	}
	if !expired && !stale && now.Sub(verified) >= s.opts.TTL {
		// The plain TTL would have refetched this answer.
		tr.Add(ctx, refresh.CountSaved, 1)
	}
	if state == "hit" {
		s.hits.Add(1)
	}
	s.respond(w, q, cached, state == "stale", state)
}

// revalidate refreshes q in the background, single flight, at most a few
// at a time; a full queue skips (the next request tries again).
func (s *Server) revalidate(q model.Query) {
	select {
	case s.background <- struct{}{}:
	default:
		return
	}
	go func() {
		defer func() { <-s.background }()
		_, _, _ = s.load(context.Background(), q, true)
	}()
}

// SetRefreshSnapshot sets the refresh stats source after the runner exists.
func (s *Server) SetRefreshSnapshot(f func(context.Context) refresh.Stats) {
	s.mu.Lock()
	s.opts.RefreshSnapshot = f
	s.mu.Unlock()
}

// Revalidate refetches one tracked row now (the refresh runner's refetch).
func (s *Server) Revalidate(ctx context.Context, it refresh.Item) error {
	var q model.Query
	if err := json.Unmarshal([]byte(it.Query), &q); err != nil || q.Validate() != nil {
		if s.opts.Refresh != nil {
			s.opts.Refresh.Forget(ctx, []string{it.Key})
		}
		return nil
	}
	// The slim copy answers this row, so the stored one is never read:
	// stop tracking it rather than fetch it live again.
	if _, ok := s.slim(ctx, q); ok {
		if s.opts.Refresh != nil {
			s.opts.Refresh.Forget(ctx, []string{it.Key})
		}
		return nil
	}
	_, _, err := s.load(ctx, q, true)
	var fe *model.FetchError
	if errors.As(err, &fe) && fe.Code == "upstream_budget" {
		return err
	}
	return nil
}

func (s *Server) load(ctx context.Context, q model.Query, force bool) (model.Entry, bool, error) {
	key := q.Key()
	s.mu.Lock()
	f := s.flights[key]
	if f == nil {
		f = &flight{done: make(chan struct{})}
		s.flights[key] = f
		go func() {
			work, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.opts.LookupTimeout)
			defer cancel()
			e, ok, err := s.store.Get(work, key)
			if err == nil && ok && !force && time.Now().Before(e.ExpiresAt) {
				f.entry = e
				f.persisted = true
			} else if err != nil {
				f.err = err
			} else {
				fctx, via := upstream.WithVia(work)
				result, fetchErr := s.fetcher.Fetch(fctx, q)
				if fetchErr != nil {
					f.err = fetchErr
				} else if !result.Complete || !validObject(result.Data) {
					f.err = &model.FetchError{Code: "upstream_incomplete", Status: 503, RetryAfter: time.Second}
				} else {
					now := time.Now().UTC()
					if q.Provider == "musicbrainz" && q.Operation == "detail" {
						result.Data = withLiveTag(result.Data, q.ID, now)
						if s.opts.LiveTag != nil {
							s.opts.LiveTag(q.Kind, strings.ToLower(q.ID), srctag.Tag{Source: "mb-live", Ref: strings.ToLower(q.ID), FetchedAt: now, ViaIP: via()})
						}
					}
					f.entry = model.Entry{Key: key, Result: result, FetchedAt: now, ExpiresAt: now.Add(s.expiry(q, result))}
					if s.store.Put(work, f.entry) == nil {
						f.persisted = true
						if s.opts.Refresh != nil {
							raw, _ := json.Marshal(q)
							_ = s.opts.Refresh.Record(work, key, string(raw), refresh.RefOf(q))
						}
						if s.opts.Stats != nil {
							s.opts.Stats.StoredItem(key, itemKind(q), q.Provider, itemTitle(result.Data))
						}
					} else {
						s.writeFailures.Add(1)
					}
				}
			}
			s.mu.Lock()
			delete(s.flights, key)
			close(f.done)
			s.mu.Unlock()
		}()
	}
	s.mu.Unlock()
	select {
	case <-f.done:
		return f.entry, f.persisted, f.err
	case <-ctx.Done():
		return model.Entry{}, false, &model.FetchError{Code: "cancelled", Status: 503, RetryAfter: time.Second}
	}
}

func (s *Server) respond(w http.ResponseWriter, q model.Query, e model.Entry, stale bool, cache string) {
	w.Header().Set("X-Metadata-Cache", cache)
	if q.Operation == model.OperationTMDB {
		// TMDB's own JSON, so the app's TMDB client reads it unchanged.
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Metadata-Fetched-At", e.FetchedAt.UTC().Format(time.RFC3339))
		if stale {
			w.Header().Set("X-Metadata-Stale", "true")
		}
		w.WriteHeader(200)
		_, _ = w.Write(e.Result.Data)
		return
	}
	if q.Provider == "musicbrainz" && q.Kind == "artist" && q.Operation != "search" {
		e.Result.Data = s.withVideos(q.ID, e.Result.Data)
		e.Result.Data = s.withPopularity(q.ID, e.Result.Data)
		var listPending bool
		e.Result.Data, listPending = s.withDiscography(q.ID, e.Result.Data)
		if s.videosPending(q.ID) || listPending {
			w.Header().Set(partialHeader, "true")
		}
	}
	if q.Provider == "musicbrainz" && (q.Kind == "artist" || q.Kind == "release-group") && q.Operation != "search" {
		e.Result.Data = s.withPictures(q.Kind, q.ID, e.Result.Data)
		if s.opts.PicturePending != nil && s.opts.PicturePending(context.Background(), q.Kind, strings.ToLower(q.ID)) {
			w.Header().Set(partialHeader, "true")
		}
	}
	writeJSON(w, 200, map[string]any{"schema_version": 1, "source": q.Provider, "source_id": q.ID, "fetched_at": e.FetchedAt, "expires_at": e.ExpiresAt, "stale": stale, "completeness": "complete", "next_cursor": e.Result.NextCursor, "data": e.Result.Data})
}

func validObject(data json.RawMessage) bool {
	var object map[string]json.RawMessage
	return json.Unmarshal(data, &object) == nil && object != nil
}

func (s *Server) expiry(q model.Query, result model.Result) time.Duration {
	ttl := s.baseExpiry(q, result)
	if providers.TVNeedsTrailer(q, result.Data) {
		// No trailer even in the seasons: asked again soon.
		ttl = min(ttl, 15*time.Minute)
	}
	return ttl
}

func (s *Server) baseExpiry(q model.Query, result model.Result) time.Duration {
	if s.opts.Refresh != nil {
		// Feeds drive freshness; verify-after is only the backstop.
		switch ref := refresh.RefOf(q); {
		case strings.HasPrefix(ref, "tmdb:"):
			return s.opts.Backstop
		case strings.HasPrefix(ref, "musicbrainz:") && s.opts.MusicFeed:
			return s.opts.Backstop
		case strings.HasPrefix(ref, "musicbrainz:"):
			return s.opts.MusicTTL
		}
	}
	ttl := s.opts.TTL
	if v := s.opts.ProviderTTLs[q.Provider]; v > 0 {
		ttl = v
	}
	if v := s.opts.OperationTTLs[q.Provider+":"+q.Operation]; v > 0 {
		ttl = v
	}
	if q.Operation == "search" {
		var data struct {
			Items []json.RawMessage `json:"items"`
		}
		if json.Unmarshal(result.Data, &data) == nil && data.Items != nil && len(data.Items) == 0 {
			ttl = min(ttl, s.opts.NegativeTTL)
		}
	}
	return ttl
}

func parseQuery(u *url.URL) (model.Query, error) {
	v, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return model.Query{}, err
	}
	if rest, ok := strings.CutPrefix(u.Path, "/v1/tmdb/"); ok {
		q := model.Query{Provider: "tmdb", Kind: "raw", Operation: model.OperationTMDB, Path: "/" + rest, Page: 1, Limit: 20}
		if q.Params, err = model.TMDBRead(q.Path, v); err != nil {
			return q, err
		}
		return q, q.Validate()
	}
	for k, values := range v {
		if len(values) != 1 || (k != "provider" && k != "kind" && k != "q" && k != "language" && k != "region" && k != "page" && k != "limit") {
			return model.Query{}, errors.New("unsupported parameter")
		}
	}
	q := model.Query{Page: 1, Limit: 20, Language: v.Get("language"), Region: v.Get("region")}
	for k, dst := range map[string]*int{"page": &q.Page, "limit": &q.Limit} {
		if raw := v.Get(k); raw != "" {
			n, e := strconv.Atoi(raw)
			if e != nil {
				return q, e
			}
			*dst = n
		}
	}
	p := strings.Split(strings.Trim(u.Path, "/"), "/")
	if u.Path == "/v1/search" {
		q.Provider = v.Get("provider")
		q.Kind = v.Get("kind")
		q.Q = v.Get("q")
		q.Operation = "search"
	} else {
		if v.Has("provider") || v.Has("kind") || v.Has("q") {
			return q, errors.New("unsupported parameter")
		}
		if len(p) == 4 && p[0] == "v1" && (p[1] == "movies" || p[1] == "shows") {
			q.Provider = p[2]
			q.ID = p[3]
			q.Operation = "detail"
			q.Kind = "movie"
			if p[1] == "shows" {
				q.Kind = "show"
			}
		} else if len(p) == 6 && p[0] == "v1" && p[1] == "shows" && p[4] == "seasons" {
			q.Provider = p[2]
			q.ID = p[3]
			q.Kind = "show"
			q.Operation = "season"
			q.Season, err = strconv.Atoi(p[5])
			if err != nil {
				return q, err
			}
		} else if len(p) == 5 && p[0] == "v1" && p[1] == "music" {
			q.Provider = p[2]
			q.Kind = p[3]
			q.ID = p[4]
			q.Operation = "detail"
		} else {
			return q, errors.New("unsupported route")
		}
	}
	if q.Operation != "search" && (v.Has("page") || v.Has("limit") || v.Has("region")) {
		return q, errors.New("unsupported detail variant")
	}
	if q.Provider != "tmdb" && (v.Has("language") || v.Has("region")) {
		return q, errors.New("unsupported locale")
	}
	if q.Provider == "tmdb" && q.Kind != "movie" && v.Has("region") {
		return q, errors.New("unsupported region")
	}
	if q.Operation == "search" && q.Provider != "musicbrainz" && q.Limit != 20 {
		return q, errors.New("provider uses native page size")
	}
	q.ID = strings.ToLower(q.ID)
	return q, q.Validate()
}

// serveRegister issues one new self-registered credential to an anonymous
// install. The caller already passed the client-address and admission
// gates; this adds the total and per-address-group registration budgets.
func (s *Server) serveRegister(w http.ResponseWriter, r *http.Request) {
	reg, ok := s.store.(Registrar)
	if !ok {
		problem(w, 503, "registration_unavailable", time.Minute)
		return
	}
	address := s.clientAddress(r)
	source := addressGroup(address)
	if !s.allowRegister(address) {
		s.rejected.Add(1)
		problem(w, 429, "registration_limit", time.Hour/time.Duration(s.opts.RegisterPerHour))
		return
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		problem(w, 503, "registration_unavailable", time.Minute)
		return
	}
	token := "hai_" + base64.RawURLEncoding.EncodeToString(b)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	err := reg.RegisterToken(ctx, token, source, networkOf(address), s.opts.RegisteredMax, s.opts.RegisteredIdle, s.opts.RegisteredProtect)
	if errors.Is(err, model.ErrRegistryFull) {
		s.rejected.Add(1)
		problem(w, 429, "registry_full", time.Hour)
		return
	}
	if err != nil {
		problem(w, 503, "registration_unavailable", time.Minute)
		return
	}
	writeJSON(w, 201, map[string]any{"schema_version": 1, "token": token})
}

// addressGroup is the limiter key for a client address: IPv6 by /64,
// IPv4 and anything else as is.
func addressGroup(address string) string {
	ip := net.ParseIP(address)
	if ip == nil || ip.To4() != nil {
		return address
	}
	return ip.Mask(net.CIDRMask(64, 128)).String() + "/64"
}

func (s *Server) statsAuthorized(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	return s.opts.StatsToken != "" && strings.HasPrefix(auth, "Bearer ") && subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(auth, "Bearer ")), []byte(s.opts.StatsToken)) == 1
}

type registerGroup struct {
	key string
	per float64
}

// registerGroups are the limiter keys and hourly limits for a client
// address, narrowest first: IPv4 /32 and /24; IPv6 /64, /48 and /32.
// Anything unparsable is one group of its own.
func (s *Server) registerGroups(address string) []registerGroup {
	per, network, wide := float64(s.opts.RegisterPerHour), float64(s.opts.RegisterNetworkPerHour), float64(s.opts.RegisterWidePerHour)
	ip := net.ParseIP(address)
	if ip == nil {
		return []registerGroup{{address, per}}
	}
	if v4 := ip.To4(); v4 != nil {
		return []registerGroup{{v4.String(), per}, {v4.Mask(net.CIDRMask(24, 32)).String() + "/24", network}}
	}
	return []registerGroup{{addressGroup(address), per}, {ip.Mask(net.CIDRMask(48, 128)).String() + "/48", network}, {ip.Mask(net.CIDRMask(32, 128)).String() + "/32", wide}}
}

// networkOf is the widest registration group of an address or a source
// group (IPv4 /24, IPv6 /32).
func networkOf(source string) string {
	ip := net.ParseIP(source)
	if ip == nil {
		if p, _, err := net.ParseCIDR(source); err == nil {
			ip = p
		}
	}
	if ip == nil {
		return source
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.Mask(net.CIDRMask(24, 32)).String() + "/24"
	}
	return ip.Mask(net.CIDRMask(32, 128)).String() + "/32"
}

func refill(a *allowance, limit float64, now time.Time) {
	a.tokens = min(limit, a.tokens+now.Sub(a.updated).Hours()*limit)
	a.updated = now
}

// allowRegister checks every address group of the caller (each its own
// hourly limit), then the shared budget. A network that has not
// registered within the hour may also use the reserved share once the
// shared one is spent. A full group table evicts its least recently used
// entry; a refusal creates no entries.
func (s *Server) allowRegister(address string) bool {
	now := time.Now()
	groups := s.registerGroups(address)
	s.mu.Lock()
	defer s.mu.Unlock()
	refill(&s.registered, float64(s.opts.RegisterGlobalPerHour-s.opts.RegisterReserved), now)
	refill(&s.reserved, float64(s.opts.RegisterReserved), now)
	widest := groups[len(groups)-1]
	fresh := true
	for _, g := range groups {
		if a := s.registrations[g.key]; a != nil {
			refill(a, g.per, now)
			if a.tokens < 1 {
				return false
			}
			if g.key == widest.key && a.tokens < g.per {
				fresh = false
			}
		}
	}
	pool := &s.registered
	if pool.tokens < 1 {
		if !fresh || s.reserved.tokens < 1 {
			return false
		}
		pool = &s.reserved
	}
	for _, g := range groups {
		a := s.registrations[g.key]
		if a == nil {
			s.makeRoom(now)
			a = &allowance{tokens: g.per, updated: now}
			s.registrations[g.key] = a
		}
		a.tokens--
	}
	pool.tokens--
	return true
}

// makeRoom drops registration entries idle over an hour, then the least
// recently used one, when the table is full.
func (s *Server) makeRoom(now time.Time) {
	if len(s.registrations) < s.opts.TokenSlots {
		return
	}
	oldest, oldestKey := now, ""
	for k, v := range s.registrations {
		if now.Sub(v.updated) > time.Hour {
			delete(s.registrations, k)
		} else if !v.updated.After(oldest) {
			oldest, oldestKey = v.updated, k
		}
	}
	if len(s.registrations) >= s.opts.TokenSlots && oldestKey != "" {
		delete(s.registrations, oldestKey)
	}
}

type callerKey struct{}

// caller is the request's credential scope: registered (self-registered,
// lookup routes only) or operator, and who asks (for popularity counts).
type caller struct {
	registered bool
	who        string
}

// asker names who made the request: a network for self-registered
// credentials (many tokens, one asker), a credential digest otherwise.
func asker(r *http.Request) string {
	c, _ := r.Context().Value(callerKey{}).(caller)
	return c.who
}

func tokenKey(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:8])
}

// registeredRoute lists the routes self-registered credentials may use;
// any route not listed here is operator only.
func registeredRoute(path string) bool {
	if path == "/v1/search" || path == "/v1/lists/service-top" || path == "/v1/lists/music-trending" || path == "/v1/anime/map" {
		return true
	}
	for _, p := range []string{"/v1/movies/", "/v1/shows/", "/v1/music/", "/v1/trailers/", "/v1/wikidata/", "/v1/tmdb/", "/v1/art/", "/v1/ratings/", "/v1/img/"} {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// sourceFactor is how many tokens' worth of requests one source address
// group may make in total with self-registered credentials.
const sourceFactor = 4

// allowSource is the total limit for all self-registered credentials from
// one source group: sourceFactor times the per-credential limit. A full
// table evicts its least recently used entry.
func (s *Server) allowSource(source string) bool {
	now := time.Now()
	rate, burst := s.opts.PerMinute*sourceFactor, float64(s.opts.Burst*sourceFactor)
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.sources[source]
	if a == nil {
		if len(s.sources) >= s.opts.TokenSlots {
			oldest, oldestKey := now, ""
			for k, v := range s.sources {
				if !v.updated.After(oldest) {
					oldest, oldestKey = v.updated, k
				}
			}
			delete(s.sources, oldestKey)
		}
		a = &allowance{tokens: burst, updated: now}
		s.sources[source] = a
	}
	a.tokens = min(burst, a.tokens+now.Sub(a.updated).Minutes()*rate)
	a.updated = now
	if a.tokens < 1 {
		return false
	}
	a.tokens--
	return true
}

// WarmMusicArtist keeps one MusicBrainz artist answer (with its release
// groups) in the cache: it fetches only when none is kept, it has expired,
// or the change feed marked it stale. fetched says whether it called out.
func (s *Server) WarmMusicArtist(ctx context.Context, mbid string) (fetched bool, err error) {
	// The same query (and so the same cache key) as GET /v1/music/musicbrainz/artist/{mbid}.
	q, err := parseQuery(&url.URL{Path: "/v1/music/musicbrainz/artist/" + mbid})
	if err != nil {
		return false, err
	}
	cached, found, err := s.store.Get(ctx, q.Key())
	if err != nil {
		return false, err
	}
	if found && time.Now().Before(cached.ExpiresAt) {
		if s.opts.Refresh == nil {
			return false, nil
		}
		if _, stale, _ := s.opts.Refresh.State(ctx, cached.Key); !stale {
			return false, nil
		}
	}
	_, _, err = s.load(ctx, q, found)
	return true, err
}

// withLiveTag marks a live MusicBrainz answer with where it came from
// (srctag "mb-live", the item id and when), as the slim copy's answers
// carry "source_tag". The egress address is never served: it is kept
// only internally (Options.LiveTag).
func withLiveTag(data []byte, id string, now time.Time) []byte {
	var obj map[string]json.RawMessage
	if json.Unmarshal(data, &obj) != nil || obj == nil {
		return data
	}
	tag, err := json.Marshal(map[string]any{"source": "mb-live", "ref": strings.ToLower(id), "fetched_at": now.Format(time.RFC3339)})
	if err != nil {
		return data
	}
	obj["source_tag"] = tag
	out, err := json.Marshal(obj)
	if err != nil {
		return data
	}
	return out
}
