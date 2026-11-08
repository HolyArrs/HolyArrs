package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/features"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/ratings"
)

// The Trakt VIP access token is kept encrypted next to the Trakt client id.
const traktTokenName = "trakt_access_token"

// reviewTTL is how long one page of a source's reviews is kept.
var reviewTTL = map[string]time.Duration{"tmdb": 6 * time.Hour, "trakt": 10 * time.Minute}

// reviewSource is one place ratings and reviews come from. More sources
// (TVDB and so on) are added to reviewSources.
type reviewSource struct {
	ID     string
	Name   string
	Orders []string
	// lister returns the source's reader, or false when it is not set up.
	lister func(a *API, ctx context.Context) (metadata.ReviewLister, bool)
}

var reviewSources = []reviewSource{
	{ID: "tmdb", Name: "TMDB", Orders: []string{metadata.ReviewsNewest}, lister: func(a *API, ctx context.Context) (metadata.ReviewLister, bool) {
		return metadata.ReviewListerFor(ctx, a.s.Metadata)
	}},
	{ID: "trakt", Name: "Trakt", Orders: []string{metadata.ReviewsNewest, metadata.ReviewsHelpful}, lister: func(a *API, ctx context.Context) (metadata.ReviewLister, bool) {
		id, token := a.traktKeys(ctx)
		if id == "" || token == "" || !startup.FeatureOn(features.TraktLists) {
			return nil, false
		}
		return &metadata.Trakt{Base: a.s.TraktBase, ClientID: id, Token: token}, true
	}},
}

// traktKeys reads the Trakt client id and VIP token; empty when not set.
func (a *API) traktKeys(ctx context.Context) (string, string) {
	if a.s.Secrets == nil {
		return "", ""
	}
	id, err := a.s.Secrets.Setting(ctx, traktKeyGroup, traktKeyName)
	if err != nil {
		return "", ""
	}
	token, err := a.s.Secrets.Setting(ctx, traktKeyGroup, traktTokenName)
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
func (a *API) metadataReviews(w http.ResponseWriter, r *http.Request) {
	kind, id := r.PathValue("kind"), r.PathValue("tmdb_id")
	if kind != "movie" && kind != "show" {
		fail(w, http.StatusBadRequest, "kind must be movie or show")
		return
	}
	if n, err := strconv.ParseInt(id, 10, 64); err != nil || n <= 0 {
		fail(w, http.StatusBadRequest, "tmdb_id must be a TMDB number")
		return
	}
	q := r.URL.Query()
	page := 1
	if p := q.Get("page"); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 500 {
			fail(w, http.StatusBadRequest, "page must be 1 to 500")
			return
		}
		page = n
	}
	order := q.Get("order")
	if order == "" {
		order = metadata.ReviewsNewest
	}
	if order != metadata.ReviewsNewest && order != metadata.ReviewsHelpful {
		fail(w, http.StatusBadRequest, "order must be newest or helpful")
		return
	}
	if len(a.kindsOn(kind)) == 0 {
		fail(w, http.StatusConflict, "that part is off")
		return
	}
	if a.kidLimited(w, r) {
		return
	}
	src := q.Get("source")
	if src == "" {
		src = "tmdb"
	}
	var sources []reviewSourceView
	var chosen metadata.ReviewLister
	var orders []string
	known := false
	for _, s := range reviewSources {
		l, ok := s.lister(a, r.Context())
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
		fail(w, http.StatusBadRequest, "unknown source")
		return
	}
	if chosen == nil {
		fail(w, http.StatusNotFound, "that source is not set up")
		return
	}
	if !contains(orders, order) {
		order = orders[0]
	}
	d, ok := metadata.DiscovererFor(r.Context(), a.s.Metadata)
	if !ok {
		fail(w, http.StatusNotImplemented, "Explore is not available here")
		return
	}
	dkey := "details|" + kind + "|" + id
	de, dcached := a.discover.get(dkey)
	if !dcached || time.Since(de.at) >= discoverTTL["details"] {
		det, err := d.Details(r.Context(), kind, id)
		if err != nil {
			discoverFail(w, err)
			return
		}
		de = discoverEntry{at: time.Now(), details: &det}
		a.discover.put(dkey, de)
	}
	max := a.kidMax(r)
	if max != "" {
		rating, inLibrary := a.libraryRating(r.Context(), kind, id)
		if !inLibrary {
			rating = de.details.Rating
		}
		if !ratings.Allows(max, kind, rating) {
			fail(w, http.StatusForbidden, aboveLimit)
			return
		}
	}
	key := "reviews|" + src + "|" + kind + "|" + id + "|" + order + "|" + strconv.Itoa(page)
	e, cached := a.discover.get(key)
	if max != "" && src == "tmdb" {
		// A Kid sees TMDB's score only, which the details already hold.
		e, cached = discoverEntry{at: time.Now(), reviews: &metadata.ReviewPage{Page: page, Pages: 1}}, true
	}
	if !cached || time.Since(e.at) >= reviewTTL[src] {
		rp, err := chosen.Reviews(r.Context(), kind, id, page, order)
		switch {
		case err == nil:
			e = discoverEntry{at: time.Now(), reviews: &rp}
			a.discover.put(key, e)
		case cached && errors.Is(err, metadata.ErrRateLimited):
		case src == "trakt" && errors.Is(err, metadata.ErrTraktLocked):
			fail(w, http.StatusBadGateway, "Trakt refused the key; check the client id and VIP token in Settings")
			return
		default:
			discoverFail(w, err)
			return
		}
	}
	out := *e.reviews
	if src == "tmdb" {
		out.Score, out.Votes = de.details.Score, de.details.Votes
	}
	hidden := false
	if max != "" {
		out.Reviews, hidden = []metadata.Review{}, true
	}
	a.exploreCaching(w, r, "")
	writeJSON(w, http.StatusOK, struct {
		Source  string             `json:"source"`
		Sources []reviewSourceView `json:"sources"`
		Order   string             `json:"order"`
		metadata.ReviewPage
		ReviewsHidden bool `json:"reviews_hidden,omitempty"`
	}{Source: src, Sources: sources, Order: order, ReviewPage: out, ReviewsHidden: hidden})
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// getTraktToken answers whether the Trakt VIP token is set (masked).
func (a *API) getTraktToken(w http.ResponseWriter, r *http.Request) {
	k, err := a.s.Secrets.Setting(r.Context(), traktKeyGroup, traktTokenName)
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": secrets.Mask(k)})
}

// putTraktToken saves the Trakt VIP access token encrypted; never logged.
func (a *API) putTraktToken(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil || body.Token == "" {
		fail(w, http.StatusBadRequest, `send {"token": "<Trakt VIP access token>"}`)
		return
	}
	if err := a.s.Secrets.SetSetting(r.Context(), traktKeyGroup, traktTokenName, body.Token, actor(r)); err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": secrets.Masked})
}
