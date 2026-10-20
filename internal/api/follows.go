package api

import (
	"errors"
	"net/http"

	"github.com/CoderLuii/holyarrs/internal/asks"
)

func (a *API) followRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/my/follows", a.myFollows)
	m.HandleFunc("GET /api/v1/titles/{id}/follow", a.getFollow)
	m.HandleFunc("PUT /api/v1/titles/{id}/follow", a.putFollow)
	m.HandleFunc("DELETE /api/v1/titles/{id}/follow", a.deleteFollow)
}

func (a *API) myFollows(w http.ResponseWriter, r *http.Request) {
	fs, err := a.s.Asks.Following(r.Context(), a.person(r).UserID)
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"follows": fs})
}

func (a *API) followState(w http.ResponseWriter, r *http.Request, titleID int64) {
	on, err := a.s.Asks.IsFollowing(r.Context(), a.person(r).UserID, titleID)
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"title_id": titleID, "following": on})
}

func (a *API) getFollow(w http.ResponseWriter, r *http.Request) {
	if t, ok := a.titleID(w, r); ok {
		a.followState(w, r, t.ID)
	}
}

func (a *API) putFollow(w http.ResponseWriter, r *http.Request) {
	t, ok := a.titleID(w, r)
	if !ok {
		return
	}
	switch err := a.s.Asks.Follow(r.Context(), a.person(r), t.ID); {
	case errors.Is(err, asks.ErrNotFollowable):
		fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, asks.ErrAboveLimit):
		fail(w, http.StatusForbidden, err.Error())
	case err != nil:
		internal(w, err)
	default:
		a.followState(w, r, t.ID)
	}
}

func (a *API) deleteFollow(w http.ResponseWriter, r *http.Request) {
	t, ok := a.titleID(w, r)
	if !ok {
		return
	}
	if err := a.s.Asks.Unfollow(r.Context(), a.person(r).UserID, t.ID); err != nil {
		internal(w, err)
		return
	}
	a.followState(w, r, t.ID)
}
