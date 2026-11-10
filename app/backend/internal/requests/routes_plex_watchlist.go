package requests

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Plex watchlist sync: each person turns on having their
// Plex watchlist asked for as them, once the owner allows it in Request
// rules. These routes read nothing from Plex: the schedule does.

// WatchlistHandlers are the My account Plex watchlist routes.
type WatchlistHandlers struct {
	Store *database.Store
	Queue *jobs.Queue
	// Watchlists gives the Plex watchlist reader (read per call; nil: none).
	Watchlists func() *Watchlists
	Person     func(r *http.Request) Person
}

// Routes registers the Plex watchlist routes.
func (h *WatchlistHandlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/my/plex-watchlist", h.getMyWatchlist)
	m.HandleFunc("PUT /api/v1/my/plex-watchlist", h.putMyWatchlist)
	m.HandleFunc("POST /api/v1/my/plex-watchlist/check", h.checkMyWatchlist)
}

// watchlistView is My account's Plex watchlist card.
type watchlistView struct {
	WatchlistState
	// On: the owner allows watchlists (Request rules).
	On bool `json:"on"`
	// Everyone: the owner turned it on for everyone with a Plex sign-in.
	Everyone bool `json:"everyone"`
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

func (h *WatchlistHandlers) watchlistView(r *http.Request) (watchlistView, error) {
	ctx := r.Context()
	uid := h.Person(r).UserID
	db := h.Store.Read()
	st, err := ReadWatchlist(ctx, db, uid)
	if err != nil {
		return watchlistView{}, err
	}
	set, err := ReadSettings(ctx, db)
	if err != nil {
		return watchlistView{}, err
	}
	v := watchlistView{WatchlistState: st, On: set.PlexWatchlists, MayAsk: mayAsk(r)}
	err = db.QueryRowContext(ctx, plexLinkNameSQL, uid).Scan(&v.PlexName)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return v, err
	}
	v.Linked = err == nil
	v.CanRead = v.Linked && h.Watchlists().HasToken(ctx, uid)
	v.Everyone = set.PlexWatchlists && set.PlexWatchlistsEveryone
	if v.Everyone && !v.Chosen && v.CanRead && v.MayAsk {
		v.Movies, v.Shows = true, true // what the schedule does for them
	}
	return v, nil
}

func (h *WatchlistHandlers) getMyWatchlist(w http.ResponseWriter, r *http.Request) {
	v, err := h.watchlistView(r)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, v)
}

func (h *WatchlistHandlers) putMyWatchlist(w http.ResponseWriter, r *http.Request) {
	var body WatchlistSwitches
	if !webserver.Decode(w, r, &body, `send {"movies": true, "shows": false}`) {
		return
	}
	if !mayAsk(r) {
		webserver.Fail(w, http.StatusForbidden, "your account may not ask for titles")
		return
	}
	if body.Movies || body.Shows {
		v, err := h.watchlistView(r)
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		if !v.On {
			webserver.Fail(w, http.StatusConflict, "the owner has not turned on Plex watchlists in Request rules")
			return
		}
		if !v.CanRead {
			webserver.Fail(w, http.StatusConflict, "link your Plex account first")
			return
		}
	}
	uid := h.Person(r).UserID
	if err := SaveWatchlistSwitches(r.Context(), h.Store, uid, body); err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	if body.Movies || body.Shows {
		h.queueWatchlist(r, uid)
	}
	h.getMyWatchlist(w, r)
}

// checkMyWatchlist reads the person's watchlist now instead of at the
// next scheduled pass.
func (h *WatchlistHandlers) checkMyWatchlist(w http.ResponseWriter, r *http.Request) {
	v, err := h.watchlistView(r)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	if !v.On || !v.CanRead || !v.MayAsk || !v.Movies && !v.Shows {
		webserver.Fail(w, http.StatusConflict, "turn on your Plex watchlist first")
		return
	}
	id, ok := h.queueWatchlist(r, h.Person(r).UserID)
	if !ok {
		webserver.Fail(w, http.StatusServiceUnavailable, "could not start the check; try again")
		return
	}
	webserver.WriteJSON(w, http.StatusAccepted, map[string]any{"job_id": id})
}

func (h *WatchlistHandlers) queueWatchlist(r *http.Request, uid int64) (int64, bool) {
	if h.Queue == nil {
		return 0, false
	}
	id, _, err := h.Queue.EnqueueAt(r.Context(), KindPlexWatchlists, nil, map[string]any{"user_id": uid},
		"plex_watchlist:"+strconv.FormatInt(uid, 10), domain.Actor{Kind: "user", ID: uid}, time.Time{})
	return id, err == nil
}
