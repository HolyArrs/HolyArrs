package api

import (
	"errors"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/asks"
)

func (a *API) followRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/my/follows", a.myFollows)
	m.HandleFunc("GET /api/v1/titles/{id}/follow", a.getFollow)
	m.HandleFunc("PUT /api/v1/titles/{id}/follow", a.putFollow)
	m.HandleFunc("DELETE /api/v1/titles/{id}/follow", a.deleteFollow)
	m.HandleFunc("GET /api/v1/metadata/show/{tmdb_id}/follow", a.getShowFollow)
	m.HandleFunc("PUT /api/v1/metadata/show/{tmdb_id}/follow", a.putShowFollow)
	m.HandleFunc("DELETE /api/v1/metadata/show/{tmdb_id}/follow", a.deleteShowFollow)
	m.HandleFunc("GET /api/v1/metadata/movie/{tmdb_id}/follow", a.getMovieFollow)
	m.HandleFunc("PUT /api/v1/metadata/movie/{tmdb_id}/follow", a.putMovieFollow)
	m.HandleFunc("DELETE /api/v1/metadata/movie/{tmdb_id}/follow", a.deleteMovieFollow)
}

// Follow a movie by TMDB id: one in the Library is followed as its title;
// one not in the Library (often not out yet) waits, the person hears once
// when it lands, and the follow moves onto the Library title.

func (a *API) movieFollowState(w http.ResponseWriter, r *http.Request, tmdbID string) {
	on, err := a.s.Asks.IsFollowingMovieTMDB(r.Context(), a.person(r).UserID, tmdbID)
	if errors.Is(err, asks.ErrBadTMDBID) {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tmdb_id": tmdbID, "following": on})
}

func (a *API) getMovieFollow(w http.ResponseWriter, r *http.Request) {
	a.movieFollowState(w, r, r.PathValue("tmdb_id"))
}

func (a *API) putMovieFollow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("tmdb_id")
	switch err := a.s.Asks.FollowMovieTMDB(r.Context(), a.person(r), id); {
	case errors.Is(err, asks.ErrBadTMDBID):
		fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, asks.ErrNotFollowable):
		fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, asks.ErrAboveLimit):
		fail(w, http.StatusForbidden, err.Error())
	case err != nil:
		internal(w, err)
	default:
		a.movieFollowState(w, r, id)
	}
}

func (a *API) deleteMovieFollow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("tmdb_id")
	switch err := a.s.Asks.UnfollowMovieTMDB(r.Context(), a.person(r).UserID, id); {
	case errors.Is(err, asks.ErrBadTMDBID):
		fail(w, http.StatusBadRequest, err.Error())
	case err != nil:
		internal(w, err)
	default:
		a.movieFollowState(w, r, id)
	}
}

// Follow by TMDB id: a show not in the Library yet can be followed; the
// person hears when it lands and, from then on, about its new seasons.

func (a *API) showFollowState(w http.ResponseWriter, r *http.Request, tmdbID string, titleID int64) {
	on, err := a.s.Asks.IsFollowingShowTMDB(r.Context(), a.person(r).UserID, tmdbID)
	if errors.Is(err, asks.ErrBadTMDBID) {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	out := map[string]any{"tmdb_id": tmdbID, "following": on}
	if titleID != 0 {
		out["title_id"] = titleID
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *API) getShowFollow(w http.ResponseWriter, r *http.Request) {
	a.showFollowState(w, r, r.PathValue("tmdb_id"), 0)
}

func (a *API) putShowFollow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("tmdb_id")
	tid, err := a.s.Asks.FollowShowTMDB(r.Context(), a.person(r), id)
	switch {
	case errors.Is(err, asks.ErrBadTMDBID), errors.Is(err, asks.ErrNotFollowable):
		fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, asks.ErrAboveLimit):
		fail(w, http.StatusForbidden, err.Error())
	case err != nil:
		internal(w, err)
	default:
		a.showFollowState(w, r, id, tid)
	}
}

func (a *API) deleteShowFollow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("tmdb_id")
	err := a.s.Asks.UnfollowShowTMDB(r.Context(), a.person(r).UserID, id)
	switch {
	case errors.Is(err, asks.ErrBadTMDBID):
		fail(w, http.StatusBadRequest, err.Error())
	case err != nil:
		internal(w, err)
	default:
		a.showFollowState(w, r, id, 0)
	}
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
	switch err := a.s.Asks.FollowTitle(r.Context(), a.person(r), t.ID); {
	case errors.Is(err, asks.ErrNotFollowable):
		fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, asks.ErrAboveLimit):
		fail(w, http.StatusForbidden, err.Error())
	case err != nil:
		internal(w, err)
	default:
		a.musicv2.days.Invalidate(a.person(r).UserID)
		a.forgetMusicV7Snaps(a.person(r).UserID)
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
	a.musicv2.days.Invalidate(a.person(r).UserID)
	a.forgetMusicV7Snaps(a.person(r).UserID)
	a.followState(w, r, t.ID)
}
