package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
)

// Plex watchlist sync (roadmap #102): each person turns on having their
// Plex watchlist asked for as them, once the owner allows it in Request
// rules. These routes read nothing from Plex: the schedule does.

func (a *API) watchlistRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/my/plex-watchlist", a.getMyWatchlist)
	m.HandleFunc("PUT /api/v1/my/plex-watchlist", a.putMyWatchlist)
	m.HandleFunc("POST /api/v1/my/plex-watchlist/check", a.checkMyWatchlist)
}

// watchlistView is My account's Plex watchlist card.
type watchlistView struct {
	requests.WatchlistState
	// On: the owner allows watchlists (Request rules).
	On bool `json:"on"`
	// Linked: a Plex account is linked to this person; PlexName is its name.
	Linked   bool   `json:"linked"`
	PlexName string `json:"plex_name,omitempty"`
	// CanRead: HolyArrs keeps a Plex sign-in for this person to read with.
	CanRead bool `json:"can_read"`
	// MayAsk: this person may ask for titles at all.
	MayAsk bool `json:"may_ask"`
}

func mayAsk(r *http.Request) bool {
	who, ok := webserver.From(r.Context())
	return !ok || who.Has(webserver.PermAsk)
}

func (a *API) watchlistView(r *http.Request) (watchlistView, error) {
	ctx := r.Context()
	uid := a.person(r).UserID
	db := a.s.Store.Read()
	st, err := requests.ReadWatchlist(ctx, db, uid)
	if err != nil {
		return watchlistView{}, err
	}
	set, err := requests.ReadSettings(ctx, db)
	if err != nil {
		return watchlistView{}, err
	}
	v := watchlistView{WatchlistState: st, On: set.PlexWatchlists, MayAsk: mayAsk(r)}
	err = db.QueryRowContext(ctx, "SELECT external_name FROM user_links WHERE provider = 'plex' AND user_id = ? LIMIT 1", uid).Scan(&v.PlexName)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return v, err
	}
	v.Linked = err == nil
	v.CanRead = v.Linked && a.s.Watchlists.HasToken(ctx, uid)
	return v, nil
}

func (a *API) getMyWatchlist(w http.ResponseWriter, r *http.Request) {
	v, err := a.watchlistView(r)
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (a *API) putMyWatchlist(w http.ResponseWriter, r *http.Request) {
	var body requests.WatchlistSwitches
	if !decode(w, r, &body, `send {"movies": true, "shows": false}`) {
		return
	}
	if !mayAsk(r) {
		fail(w, http.StatusForbidden, "your account may not ask for titles")
		return
	}
	if body.Movies || body.Shows {
		v, err := a.watchlistView(r)
		if err != nil {
			internal(w, err)
			return
		}
		if !v.On {
			fail(w, http.StatusConflict, "the owner has not turned on Plex watchlists in Request rules")
			return
		}
		if !v.CanRead {
			fail(w, http.StatusConflict, "link your Plex account first")
			return
		}
	}
	uid := a.person(r).UserID
	if err := requests.SaveWatchlistSwitches(r.Context(), a.s.Store, uid, body); err != nil {
		internal(w, err)
		return
	}
	if body.Movies || body.Shows {
		a.queueWatchlist(r, uid)
	}
	a.getMyWatchlist(w, r)
}

// checkMyWatchlist reads the person's watchlist now instead of at the
// next scheduled pass.
func (a *API) checkMyWatchlist(w http.ResponseWriter, r *http.Request) {
	v, err := a.watchlistView(r)
	if err != nil {
		internal(w, err)
		return
	}
	if !v.On || !v.CanRead || !v.MayAsk || !v.Movies && !v.Shows {
		fail(w, http.StatusConflict, "turn on your Plex watchlist first")
		return
	}
	id, ok := a.queueWatchlist(r, a.person(r).UserID)
	if !ok {
		fail(w, http.StatusServiceUnavailable, "could not start the check; try again")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": id})
}

func (a *API) queueWatchlist(r *http.Request, uid int64) (int64, bool) {
	if a.s.Queue == nil {
		return 0, false
	}
	id, _, err := a.s.Queue.EnqueueAt(r.Context(), requests.KindPlexWatchlists, nil, map[string]any{"user_id": uid},
		"plex_watchlist:"+strconv.FormatInt(uid, 10), domain.Actor{Kind: "user", ID: uid}, time.Time{})
	return id, err == nil
}
