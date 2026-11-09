package requests

import (
	"errors"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// FollowHandlers are the follow routes: titles in the Library, and shows
// and movies by TMDB id before they land.
type FollowHandlers struct {
	Svc    *Service
	Person func(r *http.Request) Person
	// Title reads the {id} title of the path, answering 404 itself.
	Title func(w http.ResponseWriter, r *http.Request) (mediafiles.Title, bool)
	// Changed runs after a person's Library follows change.
	Changed func(userID int64)
}

// Routes registers the follow routes.
func (h *FollowHandlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/my/follows", h.myFollows)
	m.HandleFunc("GET /api/v1/titles/{id}/follow", h.getFollow)
	m.HandleFunc("PUT /api/v1/titles/{id}/follow", h.putFollow)
	m.HandleFunc("DELETE /api/v1/titles/{id}/follow", h.deleteFollow)
	m.HandleFunc("GET /api/v1/metadata/show/{tmdb_id}/follow", h.getShowFollow)
	m.HandleFunc("PUT /api/v1/metadata/show/{tmdb_id}/follow", h.putShowFollow)
	m.HandleFunc("DELETE /api/v1/metadata/show/{tmdb_id}/follow", h.deleteShowFollow)
	m.HandleFunc("GET /api/v1/metadata/movie/{tmdb_id}/follow", h.getMovieFollow)
	m.HandleFunc("PUT /api/v1/metadata/movie/{tmdb_id}/follow", h.putMovieFollow)
	m.HandleFunc("DELETE /api/v1/metadata/movie/{tmdb_id}/follow", h.deleteMovieFollow)
}

// Follow a movie by TMDB id: one in the Library is followed as its title;
// one not in the Library (often not out yet) waits, the person hears once
// when it lands, and the follow moves onto the Library title.

func (h *FollowHandlers) movieFollowState(w http.ResponseWriter, r *http.Request, tmdbID string) {
	on, err := h.Svc.IsFollowingMovieTMDB(r.Context(), h.Person(r).UserID, tmdbID)
	if errors.Is(err, ErrBadTMDBID) {
		webserver.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		webserver.Internal(w, "requests", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"tmdb_id": tmdbID, "following": on})
}

func (h *FollowHandlers) getMovieFollow(w http.ResponseWriter, r *http.Request) {
	h.movieFollowState(w, r, r.PathValue("tmdb_id"))
}

func (h *FollowHandlers) putMovieFollow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("tmdb_id")
	switch err := h.Svc.FollowMovieTMDB(r.Context(), h.Person(r), id); {
	case errors.Is(err, ErrBadTMDBID):
		webserver.Fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrNotFollowable):
		webserver.Fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrAboveLimit):
		webserver.Fail(w, http.StatusForbidden, err.Error())
	case err != nil:
		webserver.Internal(w, "requests", err)
	default:
		h.movieFollowState(w, r, id)
	}
}

func (h *FollowHandlers) deleteMovieFollow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("tmdb_id")
	switch err := h.Svc.UnfollowMovieTMDB(r.Context(), h.Person(r).UserID, id); {
	case errors.Is(err, ErrBadTMDBID):
		webserver.Fail(w, http.StatusBadRequest, err.Error())
	case err != nil:
		webserver.Internal(w, "requests", err)
	default:
		h.movieFollowState(w, r, id)
	}
}

// Follow by TMDB id: a show not in the Library yet can be followed; the
// person hears when it lands and, from then on, about its new seasons.

func (h *FollowHandlers) showFollowState(w http.ResponseWriter, r *http.Request, tmdbID string, titleID int64) {
	on, err := h.Svc.IsFollowingShowTMDB(r.Context(), h.Person(r).UserID, tmdbID)
	if errors.Is(err, ErrBadTMDBID) {
		webserver.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		webserver.Internal(w, "requests", err)
		return
	}
	out := map[string]any{"tmdb_id": tmdbID, "following": on}
	if titleID != 0 {
		out["title_id"] = titleID
	}
	webserver.WriteJSON(w, http.StatusOK, out)
}

func (h *FollowHandlers) getShowFollow(w http.ResponseWriter, r *http.Request) {
	h.showFollowState(w, r, r.PathValue("tmdb_id"), 0)
}

func (h *FollowHandlers) putShowFollow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("tmdb_id")
	tid, err := h.Svc.FollowShowTMDB(r.Context(), h.Person(r), id)
	switch {
	case errors.Is(err, ErrBadTMDBID), errors.Is(err, ErrNotFollowable):
		webserver.Fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrAboveLimit):
		webserver.Fail(w, http.StatusForbidden, err.Error())
	case err != nil:
		webserver.Internal(w, "requests", err)
	default:
		h.showFollowState(w, r, id, tid)
	}
}

func (h *FollowHandlers) deleteShowFollow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("tmdb_id")
	err := h.Svc.UnfollowShowTMDB(r.Context(), h.Person(r).UserID, id)
	switch {
	case errors.Is(err, ErrBadTMDBID):
		webserver.Fail(w, http.StatusBadRequest, err.Error())
	case err != nil:
		webserver.Internal(w, "requests", err)
	default:
		h.showFollowState(w, r, id, 0)
	}
}

func (h *FollowHandlers) myFollows(w http.ResponseWriter, r *http.Request) {
	fs, err := h.Svc.Following(r.Context(), h.Person(r).UserID)
	if err != nil {
		webserver.Internal(w, "requests", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"follows": fs})
}

func (h *FollowHandlers) followState(w http.ResponseWriter, r *http.Request, titleID int64) {
	on, err := h.Svc.IsFollowing(r.Context(), h.Person(r).UserID, titleID)
	if err != nil {
		webserver.Internal(w, "requests", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"title_id": titleID, "following": on})
}

func (h *FollowHandlers) getFollow(w http.ResponseWriter, r *http.Request) {
	if t, ok := h.Title(w, r); ok {
		h.followState(w, r, t.ID)
	}
}

func (h *FollowHandlers) putFollow(w http.ResponseWriter, r *http.Request) {
	t, ok := h.Title(w, r)
	if !ok {
		return
	}
	switch err := h.Svc.FollowTitle(r.Context(), h.Person(r), t.ID); {
	case errors.Is(err, ErrNotFollowable):
		webserver.Fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrAboveLimit):
		webserver.Fail(w, http.StatusForbidden, err.Error())
	case err != nil:
		webserver.Internal(w, "requests", err)
	default:
		h.Changed(h.Person(r).UserID)
		h.followState(w, r, t.ID)
	}
}

func (h *FollowHandlers) deleteFollow(w http.ResponseWriter, r *http.Request) {
	t, ok := h.Title(w, r)
	if !ok {
		return
	}
	if err := h.Svc.Unfollow(r.Context(), h.Person(r).UserID, t.ID); err != nil {
		webserver.Internal(w, "requests", err)
		return
	}
	h.Changed(h.Person(r).UserID)
	h.followState(w, r, t.ID)
}
