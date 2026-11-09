package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// The Trakt client id and VIP access token are kept encrypted together.
const (
	TraktKeyGroup  = "links"
	TraktKeyName   = "trakt_client_id"
	traktTokenName = "trakt_access_token"
)

// reviewTTL is how long one page of a source's reviews is kept.
var reviewTTL = map[string]time.Duration{"tmdb": 6 * time.Hour, "trakt": 10 * time.Minute}

// ReviewCache keeps pages of reviews; the caller owns the memory.
type ReviewCache interface {
	GetReviews(key string) (*ReviewPage, time.Time, bool)
	PutReviews(key string, p *ReviewPage)
}

// ReviewHandlers serves the ratings and reviews routes and the Trakt VIP
// token setting. What belongs to other modules comes in as functions.
type ReviewHandlers struct {
	Provider  Provider
	Secrets   *secrets.Vault
	TraktBase string
	// TraktOn says the Trakt feature is switched on.
	TraktOn func() bool
	// PartOn says the part for a title kind (movie, show) is on.
	PartOn func(kind string) bool
	// KidLimited answers 429 and returns true when a Kid is over the limit.
	KidLimited func(w http.ResponseWriter, r *http.Request) bool
	// KidMax is the signed-in person's age-rating limit; "" for none.
	KidMax func(r *http.Request) string
	// LibraryRating is the Library title's rating; false when not there.
	LibraryRating func(ctx context.Context, kind, tmdbID string) (string, bool)
	// Details returns the title's details, cached by the caller.
	Details func(ctx context.Context, d Discoverer, kind, id string) (*Details, error)
	Cache   ReviewCache
	// Caching sets the Vary and Cache-Control headers of an Explore answer.
	Caching func(w http.ResponseWriter, r *http.Request)
	// Fail answers a provider error.
	Fail  func(w http.ResponseWriter, err error)
	Actor func(r *http.Request) database.Actor
}

// Routes registers the reviews route.
func (h *ReviewHandlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/metadata/{kind}/{tmdb_id}/reviews", h.metadataReviews)
}

// TokenRoutes registers the Trakt VIP token setting.
func (h *ReviewHandlers) TokenRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/settings/trakt-token", h.getTraktToken)
	m.HandleFunc("PUT /api/v1/settings/trakt-token", h.putTraktToken)
}

// reviewSource is one place ratings and reviews come from. More sources
// (TVDB and so on) are added to reviewSources.
type reviewSource struct {
	ID     string
	Name   string
	Orders []string
	// lister returns the source's reader, or false when it is not set up.
	lister func(h *ReviewHandlers, ctx context.Context) (ReviewLister, bool)
}

var reviewSources = []reviewSource{
	{ID: "tmdb", Name: "TMDB", Orders: []string{ReviewsNewest}, lister: func(h *ReviewHandlers, ctx context.Context) (ReviewLister, bool) {
		return ReviewListerFor(ctx, h.Provider)
	}},
	{ID: "trakt", Name: "Trakt", Orders: []string{ReviewsNewest, ReviewsHelpful}, lister: func(h *ReviewHandlers, ctx context.Context) (ReviewLister, bool) {
		id, token := h.traktKeys(ctx)
		if id == "" || token == "" || h.TraktOn == nil || !h.TraktOn() {
			return nil, false
		}
		return &Trakt{Base: h.TraktBase, ClientID: id, Token: token}, true
	}},
}

// traktKeys reads the Trakt client id and VIP token; empty when not set.
func (h *ReviewHandlers) traktKeys(ctx context.Context) (string, string) {
	if h.Secrets == nil {
		return "", ""
	}
	id, err := h.Secrets.Setting(ctx, TraktKeyGroup, TraktKeyName)
	if err != nil {
		return "", ""
	}
	token, err := h.Secrets.Setting(ctx, TraktKeyGroup, traktTokenName)
	if err != nil {
		return "", ""
	}
	return id, token
}

type reviewSourceView struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Orders []string `json:"orders"`
}

// metadataReviews answers GET /api/v1/metadata/{kind}/{tmdb_id}/reviews:
// one source's score and one page of its written reviews, with the
// sources that are set up. TMDB is always there; Trakt only once the
// owner saved its key and VIP token. A Kid gets the score but no reviews.
func (h *ReviewHandlers) metadataReviews(w http.ResponseWriter, r *http.Request) {
	kind, id := r.PathValue("kind"), r.PathValue("tmdb_id")
	if kind != "movie" && kind != "show" {
		webserver.Fail(w, http.StatusBadRequest, "kind must be movie or show")
		return
	}
	if n, err := strconv.ParseInt(id, 10, 64); err != nil || n <= 0 {
		webserver.Fail(w, http.StatusBadRequest, "tmdb_id must be a TMDB number")
		return
	}
	q := r.URL.Query()
	page := 1
	if p := q.Get("page"); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 500 {
			webserver.Fail(w, http.StatusBadRequest, "page must be 1 to 500")
			return
		}
		page = n
	}
	order := q.Get("order")
	if order == "" {
		order = ReviewsNewest
	}
	if order != ReviewsNewest && order != ReviewsHelpful {
		webserver.Fail(w, http.StatusBadRequest, "order must be newest or helpful")
		return
	}
	if !h.PartOn(kind) {
		webserver.Fail(w, http.StatusConflict, "that part is off")
		return
	}
	if h.KidLimited(w, r) {
		return
	}
	src := q.Get("source")
	if src == "" {
		src = "tmdb"
	}
	var sources []reviewSourceView
	var chosen ReviewLister
	var orders []string
	known := false
	for _, s := range reviewSources {
		l, ok := s.lister(h, r.Context())
		if s.ID == src {
			known = true
		}
		if !ok {
			continue
		}
		sources = append(sources, reviewSourceView{ID: s.ID, Name: s.Name, Orders: s.Orders})
		if s.ID == src {
			chosen, orders = l, s.Orders
		}
	}
	if !known {
		webserver.Fail(w, http.StatusBadRequest, "unknown source")
		return
	}
	if chosen == nil {
		webserver.Fail(w, http.StatusNotFound, "that source is not set up")
		return
	}
	if !slices.Contains(orders, order) {
		order = orders[0]
	}
	d, ok := DiscovererFor(r.Context(), h.Provider)
	if !ok {
		webserver.Fail(w, http.StatusNotImplemented, "Explore is not available here")
		return
	}
	det, err := h.Details(r.Context(), d, kind, id)
	if err != nil {
		h.Fail(w, err)
		return
	}
	max := h.KidMax(r)
	if max != "" {
		rating, inLibrary := h.LibraryRating(r.Context(), kind, id)
		if !inLibrary {
			rating = det.Rating
		}
		if !RatingAllows(max, kind, rating) {
			webserver.Fail(w, http.StatusForbidden, AboveLimit)
			return
		}
	}
	key := "reviews|" + src + "|" + kind + "|" + id + "|" + order + "|" + strconv.Itoa(page)
	rp, at, cached := h.Cache.GetReviews(key)
	if max != "" && src == "tmdb" {
		// A Kid sees TMDB's score only, which the details already hold.
		rp, at, cached = &ReviewPage{Page: page, Pages: 1}, time.Now(), true
	}
	if !cached || time.Since(at) >= reviewTTL[src] {
		fresh, err := chosen.Reviews(r.Context(), kind, id, page, order)
		switch {
		case err == nil:
			rp = &fresh
			h.Cache.PutReviews(key, rp)
		case cached && errors.Is(err, ErrRateLimited):
		case src == "trakt" && errors.Is(err, ErrTraktLocked):
			webserver.Fail(w, http.StatusBadGateway, "Trakt refused the key; check the client id and VIP token in Settings")
			return
		default:
			h.Fail(w, err)
			return
		}
	}
	out := *rp
	if src == "tmdb" {
		out.Score, out.Votes = det.Score, det.Votes
	}
	hidden := false
	if max != "" {
		out.Reviews, hidden = []Review{}, true
	}
	h.Caching(w, r)
	webserver.WriteJSON(w, http.StatusOK, struct {
		Source  string             `json:"source"`
		Sources []reviewSourceView `json:"sources"`
		Order   string             `json:"order"`
		ReviewPage
		ReviewsHidden bool `json:"reviews_hidden,omitempty"`
	}{Source: src, Sources: sources, Order: order, ReviewPage: out, ReviewsHidden: hidden})
}

// getTraktToken answers whether the Trakt VIP token is set (masked).
func (h *ReviewHandlers) getTraktToken(w http.ResponseWriter, r *http.Request) {
	k, err := h.Secrets.Setting(r.Context(), TraktKeyGroup, traktTokenName)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]string{"token": secrets.Mask(k)})
}

// putTraktToken saves the Trakt VIP access token encrypted; never logged.
func (h *ReviewHandlers) putTraktToken(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil || body.Token == "" {
		webserver.Fail(w, http.StatusBadRequest, `send {"token": "<Trakt VIP access token>"}`)
		return
	}
	if err := h.Secrets.SetSetting(r.Context(), TraktKeyGroup, traktTokenName, body.Token, h.Actor(r)); err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]string{"token": secrets.Masked})
}
